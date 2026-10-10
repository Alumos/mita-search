package main

import (
	"fmt"
	"regexp"
	"strconv"
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

// 既有知识库保持不变，检索时跳过教学说明条目，不提供预制答案。
func (kb *KnowledgeBase) taskReferences(query, mode string) []ScoredItem {
	ranked := kb.Search(query, len(kb.Items))
	refs := []ScoredItem{}
	seen := map[string]bool{}
	add := func(item KnowledgeItem, score float64) {
		if item.ID == "zuoyizuo-table" || seen[item.ID] {
			return
		}
		// 只调整展示用副本，知识库文件和原始条目保持不变。
		item.Title = strings.NewReplacer("（课本重点）", "", "课本", "学习任务").Replace(item.Title)
		item.Category = strings.ReplaceAll(item.Category, "课本", "学习")
		item.Content = strings.ReplaceAll(item.Content, "课本", "学习任务")
		item.Keywords = nil
		refs = append(refs, ScoredItem{Item: item, Score: score})
		seen[item.ID] = true
	}
	for _, r := range ranked {
		add(r.Item, r.Score)
		if len(refs) >= topKForMode(mode) {
			break
		}
	}
	// 所有已有事实条目均可供模型选材，不再被少量结构化模板素材限制。
	for _, item := range kb.Items {
		add(item, 0)
	}
	return refs
}

var tableStart = regexp.MustCompile(`(?m)^[ \t]*\|`)
var citation = regexp.MustCompile(`\[(\d+)\]`)

func parseTableRow(line string) []string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
		return nil
	}
	cells := strings.Split(strings.TrimSuffix(strings.TrimPrefix(line, "|"), "|"), "|")
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells
}

// 模型负责生成所有内容；这里只合并意外重复的三列表格，不补写固定答案。
func normalizeAnswer(raw string, referenceCount int) (string, error) {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	prose := []string{}
	rows := []string{}
	seen := map[string]bool{}
	core := [2]bool{}
	for i := 0; i < len(lines); {
		if !strings.HasPrefix(strings.TrimSpace(lines[i]), "|") {
			prose = append(prose, lines[i])
			i++
			continue
		}
		header := parseTableRow(lines[i])
		if len(header) != 3 || strings.Join(header, "|") != "自主建造|自主研发|优点" {
			return "", fmt.Errorf("回答包含非目标表格")
		}
		i++
		if i >= len(lines) {
			return "", fmt.Errorf("表格缺少分隔行")
		}
		separator := parseTableRow(lines[i])
		if len(separator) != 3 {
			return "", fmt.Errorf("表格分隔行无效")
		}
		for _, c := range separator {
			if len(strings.Trim(c, ":")) < 3 || strings.Trim(c, "-:") != "" {
				return "", fmt.Errorf("表格分隔行无效")
			}
		}
		i++
		for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|") {
			cells := parseTableRow(lines[i])
			if strings.Join(cells, "|") == "自主建造|自主研发|优点" {
				break // 连续的第二张表可能没有空行。
			}
			i++
			if len(cells) != 3 {
				return "", fmt.Errorf("表格行不完整")
			}
			for _, c := range cells {
				if c == "" || c == "待补充" || c == "同上" {
					return "", fmt.Errorf("表格含空白或占位答案")
				}
			}
			for _, m := range citation.FindAllStringSubmatch(strings.Join(cells, " "), -1) {
				n, _ := strconv.Atoi(m[1])
				if n < 1 || n > referenceCount {
					return "", fmt.Errorf("表格来源编号无效")
				}
			}
			key := strings.ReplaceAll(citation.ReplaceAllString(strings.Join(cells, "|"), ""), "**", "")
			if seen[key] {
				continue
			}
			seen[key] = true
			if strings.Contains(cells[0], "网络控制系统") && strings.Contains(cells[1], "车体") && strings.Contains(cells[2], "安全") {
				core[0] = true
			}
			if strings.Contains(cells[0], "牵引传动系统") && strings.Contains(cells[1], "转向架") && strings.Contains(cells[2], "智能") {
				core[1] = true
			}
			rows = append(rows, "| "+strings.Join(cells, " | ")+" |")
		}
	}
	if len(rows) < 3 || !core[0] || !core[1] {
		return "", fmt.Errorf("回答未保留已有示例并提供补充答案")
	}
	answer := strings.TrimSpace(strings.Join(prose, "\n")) + "\n\n| 自主建造 | 自主研发 | 优点 |\n| --- | --- | --- |\n" + strings.Join(rows, "\n") + "\n"
	if strings.Contains(answer, "课本") || strings.Contains(answer, "教材") {
		return "", fmt.Errorf("回答包含不适合展示的教学提示")
	}
	return answer, nil
}
