package workbench

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"novel/internal/job"
)

// Keys exist only in this write-only request DTO and in provider transports.
// Public configuration and queued task snapshots never carry secret values.
type ProviderSettingsInput struct {
	ProviderConfig
	APIKey      string `json:"apiKey,omitempty"`
	ClearAPIKey bool   `json:"clearAPIKey,omitempty"`
}
type ProviderSettingPublic struct {
	ProviderConfig
	StoredHeaderNames []string `json:"storedHeaderNames"`
}
type ProviderSettingsView struct {
	Providers []ProviderSettingPublic `json:"providers"`
	Active    ProviderSettingPublic   `json:"active"`
	Storage   string                  `json:"storage"`
}
type ProviderModelEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type ProviderDiscovery struct {
	Reachable             bool                 `json:"reachable"`
	StatusCode            int                  `json:"statusCode"`
	Models                []ProviderModelEntry `json:"models"`
	Message               string               `json:"message"`
	CredentialsConfigured bool                 `json:"credentialsConfigured"`
}

var providerKeyMu sync.Mutex

func validSecretReference(value string) bool {
	if !strings.HasPrefix(value, "secret:") || len(value) != 39 {
		return false
	}
	for _, c := range value[7:] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
func providerCredentialState(reference string) (bool, string) {
	if validSecretReference(reference) {
		return true, "stored"
	}
	if reference != "" {
		return strings.TrimSpace(os.Getenv(reference)) != "", "environment"
	}
	return false, "none"
}
func providerAdmin(ctx context.Context) error {
	p, ok := PrincipalFromContext(ctx)
	if !ok || !p.Admin {
		return ErrForbidden
	}
	return nil
}

func providerMasterKey(create bool) ([]byte, error) {
	providerKeyMu.Lock()
	defer providerKeyMu.Unlock()
	path := os.Getenv("PROVIDER_MASTER_KEY_PATH")
	if path == "" {
		path = filepath.Join(".local", "provider-master.key")
	}
	if raw, err := os.ReadFile(path); err == nil {
		if len(raw) != 32 {
			return nil, fmt.Errorf("本机供应商密钥文件无效，请恢复原密钥文件")
		}
		return raw, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("无法读取本机供应商密钥文件")
	}
	if !create {
		return nil, fmt.Errorf("缺少本机供应商密钥文件，请恢复原文件或在AI设置重新输入密钥")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("无法建立供应商密钥目录")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("无法建立本机供应商密钥")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if os.IsExist(err) {
		stored, e := os.ReadFile(path)
		if e == nil && len(stored) == 32 {
			return stored, nil
		}
		return nil, fmt.Errorf("本机供应商密钥文件暂不可用")
	}
	if err != nil {
		return nil, fmt.Errorf("无法保存本机供应商密钥文件")
	}
	_, err = f.Write(raw)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return nil, fmt.Errorf("无法完整保存本机供应商密钥文件")
	}
	return raw, nil
}
func providerAEAD(create bool) (cipher.AEAD, error) {
	key, err := providerMasterKey(create)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("供应商凭据加密初始化失败")
	}
	return cipher.NewGCM(block)
}
func (s *Service) storeProviderSecret(ctx context.Context, endpoint, value string) (string, error) {
	if len(value) == 0 || len(value) > 16000 || strings.ContainsAny(value, "\r\n") {
		return "", fmt.Errorf("密钥或敏感请求头无效")
	}
	aead, err := providerAEAD(true)
	if err != nil {
		return "", err
	}
	id := job.NewID()
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", fmt.Errorf("无法加密供应商凭据")
	}
	encrypted := aead.Seal(nonce, nonce, []byte(value), []byte(id+"\n"+endpoint))
	_, err = s.DB.Exec(ctx, "INSERT INTO wb_provider_secrets(id,endpoint,ciphertext) VALUES($1,$2,$3)", id, endpoint, encrypted)
	if err != nil {
		return "", fmt.Errorf("无法保存加密供应商凭据")
	}
	return "secret:" + id, nil
}
func (s *Service) resolvedProviderSecret(ctx context.Context, reference string) (string, error) {
	if reference == "" {
		return "", nil
	}
	if !validSecretReference(reference) {
		return strings.TrimSpace(os.Getenv(reference)), nil
	}
	id := strings.TrimPrefix(reference, "secret:")
	var raw []byte
	var endpoint string
	if err := s.DB.QueryRow(ctx, "SELECT endpoint,ciphertext FROM wb_provider_secrets WHERE id=$1", id).Scan(&endpoint, &raw); err != nil {
		return "", fmt.Errorf("加密凭据不可用，请在AI设置重新输入密钥")
	}
	aead, err := providerAEAD(false)
	if err != nil {
		return "", err
	}
	if len(raw) < aead.NonceSize() {
		return "", fmt.Errorf("加密凭据损坏")
	}
	plain, err := aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], []byte(id+"\n"+endpoint))
	if err != nil {
		return "", fmt.Errorf("无法解密供应商凭据，请恢复原本机密钥文件或重新输入")
	}
	return string(plain), nil
}
func (s *Service) validateProviderSecretEndpoint(ctx context.Context, reference, endpoint string) error {
	if !validSecretReference(reference) {
		return nil
	}
	var original string
	if err := s.DB.QueryRow(ctx, "SELECT endpoint FROM wb_provider_secrets WHERE id=$1", strings.TrimPrefix(reference, "secret:")).Scan(&original); err != nil {
		return fmt.Errorf("加密凭据引用无效")
	}
	if original != endpoint {
		return fmt.Errorf("供应商地址已变化，请重新输入密钥和敏感请求头")
	}
	return nil
}
func sensitiveProviderHeader(name string) bool {
	lower := strings.ToLower(name)
	for _, part := range []string{"api-key", "token", "authorization", "secret", "credential"} {
		if strings.Contains(lower, part) {
			return true
		}
	}
	return false
}
func publicProviderSetting(p ProviderConfig) ProviderSettingPublic {
	p.HasKey, p.CredentialSource = providerCredentialState(p.APIKeyEnv)
	if validSecretReference(p.APIKeyEnv) {
		p.APIKeyEnv = ""
	}
	headers := map[string]string{}
	names := []string{}
	for name, value := range p.Headers {
		if validSecretReference(value) {
			names = append(names, name)
			continue
		}
		headers[name] = value
	}
	sort.Strings(names)
	p.Headers = headers
	return ProviderSettingPublic{ProviderConfig: p, StoredHeaderNames: names}
}
func (s *Service) ProviderSettings(ctx context.Context) (ProviderSettingsView, error) {
	out := ProviderSettingsView{Providers: []ProviderSettingPublic{}, Storage: "encrypted-local"}
	if err := providerAdmin(ctx); err != nil {
		return out, err
	}
	configs, err := s.ListProviderConfigs(ctx)
	if err != nil {
		return out, err
	}
	for _, p := range configs {
		if p.SupersededBy != "" {
			continue
		}
		out.Providers = append(out.Providers, publicProviderSetting(p))
	}
	active := ProviderConfig{ID: "environment", Name: "当前环境配置", Protocol: os.Getenv("LLM_WIRE_API"), BaseURL: os.Getenv("LLM_BASE_URL"), APIKeyEnv: "LLM_API_KEY", Model: s.Model, Enabled: true, IsDefault: true, Headers: map[string]string{}}
	if active.Protocol == "" {
		active.Protocol = "responses"
	}
	if s.ProviderName == "fake" {
		active.Name = "本地演示模型"
		active.APIKeyEnv = ""
		active.Protocol = "demo"
		active.BaseURL = ""
	}
	for _, p := range configs {
		if p.Enabled && p.IsDefault && p.SupersededBy == "" {
			active = p
			break
		}
	}
	// A task-specific Copilot route takes precedence over the general default,
	// matching the model used by both /api/config and the writing assistant.
	for _, p := range configs {
		if !p.Enabled || p.SupersededBy != "" {
			continue
		}
		for _, kind := range p.TaskTypes {
			if kind == "copilot" {
				active = p
				out.Active = publicProviderSetting(active)
				return out, nil
			}
		}
	}
	out.Active = publicProviderSetting(active)
	return out, nil
}

// Prepare a new immutable config. Empty write-only fields on an edit keep the
// former encrypted reference; explicit clear and changed endpoints are separate.
func (s *Service) prepareProviderSetting(ctx context.Context, in ProviderSettingsInput, persist bool) (ProviderConfig, []string, error) {
	p := in.ProviderConfig
	stored := []string{}
	if !validEndpoint(p.BaseURL) {
		return p, stored, fmt.Errorf("供应商URL无效")
	}
	if p.Protocol != "chat" && p.Protocol != "responses" && p.Protocol != "ollama" {
		return p, stored, fmt.Errorf("请选择Chat、Responses或Ollama协议")
	}
	if in.APIKey != "" && in.ClearAPIKey {
		return p, stored, fmt.Errorf("不能同时填写和清除密钥")
	}
	old := ProviderConfig{}
	if p.ID == "environment" {
		old = ProviderConfig{BaseURL: os.Getenv("LLM_BASE_URL"), APIKeyEnv: "LLM_API_KEY", Headers: map[string]string{}}
		if os.Getenv("LLM_ACTOR_AUTHORIZATION") != "" {
			old.Headers["x-openai-actor-authorization"] = "env:LLM_ACTOR_AUTHORIZATION"
		}
	} else if p.ID != "" {
		var err error
		old, err = s.providerConfig(ctx, p.ID)
		if err != nil {
			return p, stored, err
		}
	}
	if p.Headers == nil {
		p.Headers = map[string]string{}
	} else {
		copy := map[string]string{}
		for k, v := range p.Headers {
			copy[k] = v
		}
		p.Headers = copy
	}
	if p.ID != "" {
		if p.BaseURL != old.BaseURL && in.APIKey == "" && p.APIKeyEnv != "" && p.APIKeyEnv == old.APIKeyEnv && !in.ClearAPIKey && !p.NoAuth {
			return p, stored, fmt.Errorf("供应商地址已变化，请重新输入密钥")
		}
		if p.BaseURL != old.BaseURL {
			for name, value := range p.Headers {
				if !sensitiveProviderHeader(name) || !strings.HasPrefix(value, "env:") {
					continue
				}
				for oldName, oldValue := range old.Headers {
					if strings.EqualFold(name, oldName) && value == oldValue {
						return p, stored, fmt.Errorf("供应商地址已变化，请重新输入敏感请求头")
					}
				}
			}
		}
		if p.BaseURL == old.BaseURL {
			for name, value := range old.Headers {
				if validSecretReference(value) || (sensitiveProviderHeader(name) && strings.HasPrefix(value, "env:")) {
					if _, set := p.Headers[name]; !set {
						p.Headers[name] = value
					}
				}
			}
		}
		if in.APIKey == "" && p.APIKeyEnv == "" && !in.ClearAPIKey && !p.NoAuth {
			if p.BaseURL != old.BaseURL && old.APIKeyEnv != "" {
				return p, stored, fmt.Errorf("供应商地址已变化，请重新输入密钥")
			}
			p.APIKeyEnv = old.APIKeyEnv
		}
	}
	if in.ClearAPIKey || p.NoAuth {
		p.APIKeyEnv = ""
	}
	if in.APIKey != "" {
		if p.NoAuth {
			return p, stored, fmt.Errorf("无需认证模式不能同时填写密钥")
		}
		key := strings.TrimSpace(in.APIKey)
		if key == "" || len(key) > 16000 || strings.ContainsAny(key, "\r\n") {
			return p, stored, fmt.Errorf("API密钥无效")
		}
		if persist {
			ref, err := s.storeProviderSecret(ctx, p.BaseURL, key)
			if err != nil {
				return p, stored, err
			}
			p.APIKeyEnv = ref
			stored = append(stored, ref)
		} else {
			p.APIKeyEnv = ""
			p.HasKey = true
		}
	}
	if p.NoAuth {
		p.APIKeyEnv = ""
	}
	if p.APIKeyEnv != "" && !envReference.MatchString(p.APIKeyEnv) && !validSecretReference(p.APIKeyEnv) {
		return p, stored, fmt.Errorf("凭据引用无效")
	}
	if p.Protocol != "ollama" && !p.NoAuth && p.APIKeyEnv == "" && in.APIKey == "" {
		return p, stored, fmt.Errorf("请输入API密钥，或明确选择无需认证")
	}
	if err := s.validateProviderSecretEndpoint(ctx, p.APIKeyEnv, p.BaseURL); err != nil {
		return p, stored, err
	}
	for name, value := range p.Headers {
		if strings.ContainsAny(name+value, "\r\n") || strings.EqualFold(name, "authorization") || strings.EqualFold(name, "host") || strings.EqualFold(name, "content-type") {
			return p, stored, fmt.Errorf("请求头名称或内容无效")
		}
		if value == "" && sensitiveProviderHeader(name) {
			delete(p.Headers, name)
			continue
		}
		if validSecretReference(value) {
			if err := s.validateProviderSecretEndpoint(ctx, value, p.BaseURL); err != nil {
				return p, stored, err
			}
			continue
		}
		if strings.HasPrefix(value, "env:") {
			if !envReference.MatchString(strings.TrimPrefix(value, "env:")) {
				return p, stored, fmt.Errorf("请求头凭据引用无效")
			}
			continue
		}
		if sensitiveProviderHeader(name) && value != "" && persist {
			ref, err := s.storeProviderSecret(ctx, p.BaseURL, value)
			if err != nil {
				return p, stored, err
			}
			p.Headers[name] = ref
			stored = append(stored, ref)
		}
	}
	return p, stored, nil
}
func (s *Service) discardNewProviderSecrets(ctx context.Context, refs []string) {
	for _, ref := range refs {
		_, _ = s.DB.Exec(ctx, "DELETE FROM wb_provider_secrets WHERE id=$1", strings.TrimPrefix(ref, "secret:"))
	}
}
func (s *Service) SaveProviderSetting(ctx context.Context, in ProviderSettingsInput) (ProviderSettingPublic, error) {
	if err := providerAdmin(ctx); err != nil {
		return ProviderSettingPublic{}, err
	}
	p, stored, err := s.prepareProviderSetting(ctx, in, true)
	if err != nil {
		s.discardNewProviderSecrets(ctx, stored)
		return ProviderSettingPublic{}, err
	}
	replaces := in.ID
	if replaces == "environment" {
		replaces = ""
	}
	p, err = s.saveProviderConfigVersion(ctx, p, replaces)
	if err != nil {
		s.discardNewProviderSecrets(ctx, stored)
		return ProviderSettingPublic{}, err
	}
	return publicProviderSetting(p), nil
}
func (s *Service) SetProviderDefault(ctx context.Context, id string) error {
	if err := providerAdmin(ctx); err != nil {
		return err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext(current_database()||current_schema()||'provider-config-default'))"); err != nil {
		return err
	}
	var enabled bool
	var replacement string
	if err = tx.QueryRow(ctx, "SELECT enabled,superseded_by FROM wb_provider_configs WHERE id=$1 FOR UPDATE", id).Scan(&enabled, &replacement); err != nil {
		return noRows(err)
	}
	if !enabled {
		return fmt.Errorf("请先启用该供应商")
	}
	if replacement != "" {
		return &Conflict{Message: "该供应商配置已有新版本，请选择当前配置"}
	}
	if _, err = tx.Exec(ctx, "UPDATE wb_provider_configs SET is_default=(id=$1)", id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Service) DiscoverProviderModels(ctx context.Context, in ProviderSettingsInput) (ProviderDiscovery, error) {
	result := ProviderDiscovery{Models: []ProviderModelEntry{}}
	if err := providerAdmin(ctx); err != nil {
		return result, err
	}
	p, _, err := s.prepareProviderSetting(ctx, in, false)
	if err != nil {
		return result, err
	}
	key, err := s.resolvedProviderSecret(ctx, p.APIKeyEnv)
	if err != nil {
		return result, err
	}
	if in.APIKey != "" {
		key = strings.TrimSpace(in.APIKey)
	}
	headers, err := s.resolvedProviderHeaders(ctx, p.Headers)
	if err != nil {
		return result, err
	}
	result.CredentialsConfigured = p.NoAuth || p.Protocol == "ollama" || key != ""
	suffix := "/models"
	if p.Protocol == "ollama" {
		suffix = "/api/tags"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.BaseURL, "/")+suffix, nil)
	if err != nil {
		return result, fmt.Errorf("模型目录地址无效")
	}
	if key != "" && !p.NoAuth {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		result.Message = "无法连接供应商，请检查地址和网络"
		return result, nil
	}
	defer response.Body.Close()
	result.StatusCode = response.StatusCode
	raw, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil || len(raw) > 2<<20 {
		result.Message = "模型目录响应中断或过大"
		return result, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		result.Message = "供应商拒绝模型目录请求，请检查密钥、地址和权限"
		return result, nil
	}
	var payload struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
		Models []struct {
			Name  string `json:"name"`
			Model string `json:"model"`
		} `json:"models"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		result.Message = "接口可连接，但模型目录格式不受支持；可以手动填写模型名"
		return result, nil
	}
	seen := map[string]bool{}
	add := func(id, name string) {
		if id == "" || seen[id] || len(id) > 300 || len(result.Models) >= 1000 {
			return
		}
		if name == "" {
			name = id
		}
		seen[id] = true
		result.Models = append(result.Models, ProviderModelEntry{ID: id, Name: name})
	}
	for _, m := range payload.Data {
		add(m.ID, m.Name)
	}
	for _, m := range payload.Models {
		id := m.Model
		if id == "" {
			id = m.Name
		}
		add(id, m.Name)
	}
	sort.Slice(result.Models, func(i, j int) bool { return result.Models[i].ID < result.Models[j].ID })
	result.Reachable = true
	result.Message = "连接成功；仅获取模型目录，未生成内容"
	return result, nil
}
