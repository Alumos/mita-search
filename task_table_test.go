package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func classroomKB(t *testing.T) *KnowledgeBase {
	t.Helper()
	kb, err := loadKnowledge("knowledge/knowledge_base.json")
	if err != nil {
		t.Fatal(err)
	}
	return kb
}

const tableHeader = "| 自主建造 | 自主研发 | 优点 |\n| --- | --- | --- |\n"
const givenRows = "| 网络控制系统 | 车体 | 安全 |\n| 牵引传动系统 | 转向架 | 智能 |\n"

func TestReferencesAllowDiverseAnswersWithoutChangingKnowledge(t *testing.T) {
	kb := classroomKB(t)
	before, _ := json.Marshal(kb)
	for _, q := range []string{"智能运维", "地震预警", "软件开发平台", "节能环保"} {
		refs := kb.taskReferences(q, "brief")
		ids := map[string]bool{}
		for _, r := range refs {
			if ids[r.Item.ID] || r.Item.ID == "zuoyizuo-table" {
				t.Fatal("duplicate or teaching reference")
			}
			ids[r.Item.ID] = true
			if strings.Contains(r.Item.Title+r.Item.Content, "课本") {
				t.Fatal("teaching wording exposed")
			}
		}
		for _, id := range []string{"auto-drive", "jingzhang-ato", "smart-health-2021", "remote-maintenance", "earthquake-warning", "traction-software-platform", "customization-2023"} {
			if !ids[id] {
				t.Fatal("missing potential supplementary answer", id)
			}
		}
	}
	after, _ := json.Marshal(kb)
	if string(before) != string(after) {
		t.Fatal("knowledge was mutated")
	}
}

func TestAnswerMergesTablesAndKeepsGeneratedSupplementaryRows(t *testing.T) {
	first := tableHeader + givenRows + "| 地震紧急处置装置[1] | 仿真测试平台[2] | 远程维护[3] |\n"
	second := tableHeader + givenRows + "| 制动系统[1] | 牵引软件平台[2] | 舒适[3] |\n"
	answer, err := normalizeAnswer("# 创新\n\n讲解。\n\n"+first+"\n进一步说明。\n\n"+second, 3)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(answer, "| 自主建造 |") != 1 || strings.Count(answer, "| 网络控制系统 |") != 1 {
		t.Fatal("duplicate table or rows")
	}
	if !strings.Contains(answer, "地震紧急处置装置[1]") || !strings.Contains(answer, "牵引软件平台[2]") {
		t.Fatal("generated answers lost")
	}
	if !strings.HasSuffix(answer, "| 制动系统[1] | 牵引软件平台[2] | 舒适[3] |\n") {
		t.Fatal("table is not final")
	}
}

func TestNoFixedRowCountOrAnswerSet(t *testing.T) {
	for _, count := range []int{3, 7, 13, 25} {
		raw := tableHeader + givenRows
		for i := 2; i < count; i++ {
			raw += fmt.Sprintf("| 生成项目%d[1] | 生成研发%d[2] | 生成优点%d[3] |\n", i, i, i)
		}
		answer, err := normalizeAnswer(raw, 3)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(answer, "\n| ") != count+2 {
			t.Fatal("rows capped or replaced", count)
		}
	}
}

func TestAdjacentTablesAreMerged(t *testing.T) {
	raw := tableHeader + givenRows + tableHeader + givenRows + "| 制动[1] | 平台[2] | 舒适[3] |\n"
	answer, err := normalizeAnswer(raw, 3)
	if err != nil || strings.Count(answer, "| 自主建造 |") != 1 {
		t.Fatalf("adjacent tables were not merged: %v", err)
	}
}

func TestRejectIncompleteOrUnsupportedTableFormats(t *testing.T) {
	for _, raw := range []string{
		"# 没有表格", tableHeader + givenRows,
		tableHeader + givenRows + "| 制动 | | 舒适 |\n",
		tableHeader + givenRows + "| 制动[999] | 平台 | 舒适 |\n",
		tableHeader + givenRows + "| 制动 | 平台 | 舒适\n",
		"课本\n" + tableHeader + givenRows + "| 制动 | 平台 | 舒适 |\n",
	} {
		if _, err := normalizeAnswer(raw, 5); err == nil {
			t.Fatal("invalid answer accepted", raw)
		}
	}
}

func TestSearchStreamsExplanationAndPublishesOnlyOneGeneratedTable(t *testing.T) {
	kb := classroomKB(t)
	for _, mode := range []string{"brief", "deep", "research"} {
		t.Run(mode, func(t *testing.T) {
			raw := "# 自主创新\n\n先读讲解。\n\n" + tableHeader + givenRows + "| 地震处置[1] | 仿真平台[2] | 远程维护[3] |\n\n" + tableHeader + givenRows + "| 制动[1] | 牵引平台[2] | 舒适[3] |\n"
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req chatRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				if strings.Contains(req.Messages[1].Content, "【本次任务表") {
					t.Error("fixed table still injected")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				// Character-sized chunks split every possible table boundary.
				for _, r := range raw {
					data, _ := json.Marshal(map[string]interface{}{"choices": []interface{}{map[string]interface{}{"delta": map[string]string{"content": string(r)}}}})
					fmt.Fprintf(w, "data: %s\n\n", data)
				}
				fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer upstream.Close()
			s := &Server{kb: kb, cfg: &Config{SiliconFlow: SiliconFlowConfig{APIKey: "test", Model: "test", BaseURL: upstream.URL}}}
			w := httptest.NewRecorder()
			s.handleSearch(w, httptest.NewRequest(http.MethodPost, "/api/search", strings.NewReader(fmt.Sprintf(`{"query":"创新","mode":%q}`, mode))))
			var final string
			done := false
			for _, block := range strings.Split(w.Body.String(), "\n\n") {
				lines := strings.Split(block, "\n")
				if len(lines) < 2 {
					continue
				}
				event := strings.TrimPrefix(lines[0], "event: ")
				if event == "delta" || event == "answer" {
					var data map[string]string
					if err := json.Unmarshal([]byte(strings.TrimPrefix(lines[1], "data: ")), &data); err != nil {
						t.Fatal(err)
					}
					if event == "delta" && strings.Contains(data["t"], "|") {
						t.Fatal("table streamed before deduplication")
					}
					if event == "answer" {
						if final != "" {
							t.Fatal("multiple final answers")
						}
						final = data["t"]
					}
				}
				if event == "done" {
					done = true
				}
			}
			if !done || strings.Count(final, "| 自主建造 |") != 1 || !strings.Contains(final, "牵引平台[2]") {
				t.Fatal("missing single generated final table")
			}
		})
	}
}

func TestMissingTableRetriesInsteadOfAppendingCannedAnswer(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		text := "# 无表格\n讲解"
		if calls == 2 {
			text = "# 成果\n" + tableHeader + givenRows + "| 制动[1] | 软件平台[2] | 舒适[3] |\n"
		}
		data, _ := json.Marshal(map[string]interface{}{"choices": []interface{}{map[string]interface{}{"delta": map[string]string{"content": text}}}})
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
	}))
	defer upstream.Close()
	s := &Server{kb: classroomKB(t), cfg: &Config{SiliconFlow: SiliconFlowConfig{Model: "first", FallbackModels: []string{"second"}, BaseURL: upstream.URL}}}
	w := httptest.NewRecorder()
	s.handleSearch(w, httptest.NewRequest(http.MethodPost, "/api/search", strings.NewReader(`{"query":"复兴号"}`)))
	if calls != 2 || !strings.Contains(w.Body.String(), "event: retry") || !strings.Contains(w.Body.String(), "event: done") {
		t.Fatal("invalid output did not retry")
	}
}
