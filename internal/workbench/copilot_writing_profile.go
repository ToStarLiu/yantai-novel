package workbench

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"novel/internal/job"
)

const bookWritingProfileID = "book-writing-profile"

type copilotWritingProfile struct {
	Version int64
	Content string
}

func loadCopilotWritingProfile(ctx context.Context, db queryer, pid string) (*copilotWritingProfile, error) {
	rows, err := db.Query(ctx, "SELECT version,data FROM wb_studio_documents WHERE project_id=$1 AND kind='brief' AND id='main'", pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	var version int64
	var raw []byte
	if err = rows.Scan(&version, &raw); err != nil {
		return nil, err
	}
	var brief StoryBrief
	if err = json.Unmarshal(raw, &brief); err != nil {
		return nil, err
	}
	if strings.TrimSpace(brief.Style+brief.Pace+brief.Restrictions) == "" {
		return nil, nil
	}
	return &copilotWritingProfile{Version: version, Content: "文风与表达：\n" + brief.Style + "\n\n节奏：\n" + brief.Pace + "\n\n避免事项：\n" + brief.Restrictions}, nil
}

func (profile *copilotWritingProfile) item(pid string, offset, limit int) (map[string]any, error) {
	runes := []rune(profile.Content)
	if offset > len(runes) {
		return nil, fmt.Errorf("文风设定续读超出正文范围")
	}
	end := min(len(runes), offset+limit)
	return map[string]any{"id": bookWritingProfileID, "projectId": pid, "kind": "style", "name": "本书文风与约束", "version": profile.Version, "hash": job.Hash([]byte(profile.Content)), "content": string(runes[offset:end]), "contentOffset": offset, "nextContentOffset": end, "totalChars": len(runes), "truncated": end < len(runes), "authority": "本书创作写法与避免事项，不是已发生事实或角色知识；当前作者要求和事实边界优先。"}, nil
}

func (profile *copilotWritingProfile) directory() any {
	preview, cut := boundedCopilotText(profile.Content, 24)
	return map[string]any{"id": bookWritingProfileID, "kind": "style", "version": profile.Version, "preview": preview, "truncated": cut, "readTool": "read_settings"}
}

const bookWritingProfileAuthority = "\nwritingProfile是本书文风、节奏与避免事项目录。写作改稿前用read_settings(id=book-writing-profile)及contentOffset读完整相关写法；它不是故事事实，当前要求优先。"
