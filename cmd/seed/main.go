package main

import (
	"context"
	"fmt"
	"novel/internal/app"
	"novel/internal/evaluation"
	"novel/internal/workbench"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	ctx := context.Background()
	s, err := app.Service(ctx)
	if err != nil {
		return err
	}
	defer s.DB.Close()
	list, err := s.ListProjects(ctx)
	if err != nil {
		return err
	}
	for _, p := range list {
		if p.Title == "断桥铜印 · 示例作品" {
			fmt.Println("Example project already exists:", p.ID)
			return nil
		}
	}
	f, err := evaluation.Load("evaluation/fixtures/bridge-3")
	if err != nil {
		return err
	}
	p, err := s.CreateProject(ctx, "断桥铜印 · 示例作品", "原创三章示例。用于探索编辑、版本、记忆、引用与改稿影响；可自由修改或删除。", "悬疑奇幻")
	if err != nil {
		return err
	}
	for _, setting := range []workbench.Setting{{Kind: "character", Name: "林川", Content: "年轻旅人。左臂受伤，只知道铜印与王都有关；把它误认为商队凭牌。角色不能使用读者才知道的秘密。"}, {Kind: "character", Name: "沈月", Content: "识得铜印来历，但没有向林川透露所属机构与用途。"}, {Kind: "world", Name: "铜印", Content: "单一物品，只有一枚；需要记录每次持有人与位置变化。"}, {Kind: "world", Name: "旧桥北岸废磨坊", Content: "三章连续发生在同一个雨夜。木箱尚未破封，内容未知。"}, {Kind: "style", Name: "克制的调查叙事", Content: workbench.StyleTemplates()[2].Instructions}} {
		if _, err = s.SaveSetting(ctx, p.ID, setting); err != nil {
			return err
		}
	}
	for _, chapter := range f.Chapters {
		ch, e := s.CreateChapter(ctx, p.ID, chapter.Title, "第一卷 · 雨夜")
		if e != nil {
			return e
		}
		ch, e = s.UpdateChapter(ctx, ch.ID, ch.Title, ch.Volume, chapter.Outline)
		if e != nil {
			return e
		}
		if _, e = s.SaveRevision(ctx, ch.ID, workbench.SaveInput{ExpectedCurrentRevisionID: ch.CurrentRevisionID, Document: workbench.DocumentFromText(chapter.Text), Source: "import"}); e != nil {
			return e
		}
	}
	fmt.Println("Example project created:", p.ID)
	return nil
}
