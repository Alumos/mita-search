package main

import (
	"fmt"
	"sort"
	"strings"
)

// 每栏前两项是课本原有示例，扩展项根据提问排序。
type TaskCell struct {
	Text         string   `json:"text"`
	Detail       string   `json:"detail"`
	ReferenceIDs []string `json:"reference_ids"`
	Keywords     []string `json:"keywords"`
}

type TaskColumns struct {
	Construction []TaskCell `json:"construction"`
	Research     []TaskCell `json:"research"`
	Advantages   []TaskCell `json:"advantages"`
}

func (c TaskColumns) all() [][]TaskCell {
	return [][]TaskCell{c.Construction, c.Research, c.Advantages}
}

func (kb *KnowledgeBase) validateTaskColumns() error {
	ids := make(map[string]bool)
	for _, item := range kb.Items {
		if item.ID == "" || ids[item.ID] {
			return fmt.Errorf("知识条目 ID 为空或重复：%q", item.ID)
		}
		ids[item.ID] = true
	}
	for _, id := range []string{"zuoyizuo-table", "why-fuxing"} {
		if !ids[id] {
			return fmt.Errorf("缺少课本核心资料：%s", id)
		}
	}
	core := [][]string{{"网络控制系统", "牵引传动系统"}, {"车体", "转向架"}, {"安全", "智能"}}
	for col, cells := range kb.TaskColumns.all() {
		if len(cells) < 5 {
			return fmt.Errorf("任务表第 %d 栏至少需要5项素材", col+1)
		}
		for row, cell := range cells {
			if row < 2 && cell.Text != core[col][row] {
				return fmt.Errorf("任务表须保留课本示例：%s", core[col][row])
			}
			if strings.TrimSpace(cell.Text) == "" || strings.TrimSpace(cell.Detail) == "" || len(cell.ReferenceIDs) == 0 {
				return fmt.Errorf("任务表素材缺少名称、说明或来源：%q", cell.Text)
			}
			for _, id := range cell.ReferenceIDs {
				if !ids[id] {
					return fmt.Errorf("任务表素材 %q 引用了不存在的知识：%s", cell.Text, id)
				}
			}
		}
	}
	return nil
}

func taskRowCount(mode string) int {
	switch mode {
	case "brief":
		return 3
	case "research":
		return 5
	default:
		return 4
	}
}

func selectCells(cells []TaskCell, query string, count int) []TaskCell {
	// 复制再排序，避免并发搜索改变共享知识库。
	selected := append([]TaskCell(nil), cells[:2]...)
	extra := append([]TaskCell(nil), cells[2:]...)
	q := strings.ToLower(query)
	score := func(cell TaskCell) int {
		n := 0
		for _, word := range cell.Keywords {
			if word != "" && strings.Contains(q, strings.ToLower(word)) {
				n++
			}
		}
		return n
	}
	sort.SliceStable(extra, func(i, j int) bool { return score(extra[i]) > score(extra[j]) })
	return append(selected, extra[:count-2]...)
}

func (kb *KnowledgeBase) selectTaskColumns(query, mode string) TaskColumns {
	n := taskRowCount(mode)
	return TaskColumns{
		Construction: selectCells(kb.TaskColumns.Construction, query, n),
		Research:     selectCells(kb.TaskColumns.Research, query, n),
		Advantages:   selectCells(kb.TaskColumns.Advantages, query, n),
	}
}

func (kb *KnowledgeBase) taskReferences(query, mode string, columns TaskColumns) []ScoredItem {
	refs := kb.Search(query, topKForMode(mode))
	seen := make(map[string]bool)
	for _, ref := range refs {
		seen[ref.Item.ID] = true
	}
	byID := make(map[string]KnowledgeItem)
	for _, item := range kb.Items {
		byID[item.ID] = item
	}
	appendRef := func(id string) {
		if !seen[id] {
			refs = append(refs, ScoredItem{Item: byID[id]})
			seen[id] = true
		}
	}
	// 窄问题也需要覆盖整个任务表，不能只依赖相关性排名。
	appendRef("zuoyizuo-table")
	appendRef("why-fuxing")
	for _, cells := range columns.all() {
		for _, cell := range cells {
			for _, id := range cell.ReferenceIDs {
				appendRef(id)
			}
		}
	}
	return refs
}

func buildTaskTable(columns TaskColumns, refs []ScoredItem, mode string) string {
	numbers := make(map[string]int)
	for i, ref := range refs {
		numbers[ref.Item.ID] = i + 1
	}
	cellText := func(cell TaskCell) string {
		text := cell.Text
		if mode != "brief" {
			text += "：" + cell.Detail
		}
		text = strings.NewReplacer("|", "／", "\n", " ", "\r", " ").Replace(text)
		for _, id := range cell.ReferenceIDs {
			text += fmt.Sprintf("[%d]", numbers[id])
		}
		return text
	}
	var b strings.Builder
	b.WriteString("\n\n## 课本任务表：“复兴号”动车组列车的自主创新及其优点\n\n")
	b.WriteString("三栏分别列举，保留课本示例并补充资料；同一行不表示一一对应的因果关系。\n\n")
	b.WriteString("| 自主建造 | 自主研发 | 优点 |\n| --- | --- | --- |\n")
	for i := range columns.Construction {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", cellText(columns.Construction[i]), cellText(columns.Research[i]), cellText(columns.Advantages[i]))
	}
	return b.String()
}
