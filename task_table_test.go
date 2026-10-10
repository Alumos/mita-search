package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strconv"
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

func TestTaskTablesCoverTextbookAndSources(t *testing.T) {
	kb := classroomKB(t)
	for _, mode := range []string{"brief", "deep", "research", "unknown"} {
		for _, query := range []string{"网络控制系统是什么", "2018年京沈试验", "舒适与节能", "如何炒饭"} {
			t.Run(mode+query, func(t *testing.T) {
				columns := kb.selectTaskColumns(query, mode)
				refs := kb.taskReferences(query, mode, columns)
				md := buildTaskTable(columns, refs, mode)
				for _, word := range []string{"| 自主建造 | 自主研发 | 优点 |", "网络控制系统", "牵引传动系统", "车体", "转向架", "安全", "智能"} {
					if !strings.Contains(md, word) {
						t.Fatalf("missing textbook content: %s", word)
					}
				}
				rows := strings.Count(md, "\n| ") - 2 // header and delimiter
				if rows != taskRowCount(mode) {
					t.Fatalf("got %d rows, want %d", rows, taskRowCount(mode))
				}
				seen := map[string]bool{}
				for _, ref := range refs {
					if seen[ref.Item.ID] || ref.Item.Content == "" {
						t.Fatal("duplicate or missing reference", ref.Item.ID)
					}
					seen[ref.Item.ID] = true
				}
				for _, col := range columns.all() {
					for _, cell := range col {
						for _, id := range cell.ReferenceIDs {
							if !seen[id] {
								t.Fatal("table source not retrieved", id)
							}
						}
					}
				}
				for _, match := range regexp.MustCompile(`\[(\d+)\]`).FindAllStringSubmatch(md, -1) {
					n, _ := strconv.Atoi(match[1])
					if n < 1 || n > len(refs) {
						t.Fatal("invalid citation", match[0])
					}
				}
			})
		}
	}
}

func TestTaskExtensionsFollowQuestionWithoutMutatingKnowledge(t *testing.T) {
	kb := classroomKB(t)
	before, _ := json.Marshal(kb.TaskColumns)
	comfort := kb.selectTaskColumns("舒适、降噪", "brief")
	energy := kb.selectTaskColumns("节能环保", "brief")
	if comfort.Research[2].Text != "减振降噪设计" || comfort.Advantages[2].Text != "舒适" {
		t.Fatal("comfort question did not select relevant extensions")
	}
	if energy.Research[2].Text != "低阻力外形设计" || energy.Advantages[2].Text != "节能环保" {
		t.Fatal("energy question did not select relevant extensions")
	}
	after, _ := json.Marshal(kb.TaskColumns)
	if string(before) != string(after) {
		t.Fatal("selection mutated shared knowledge")
	}
}

func TestKnowledgeRejectsBrokenTableSources(t *testing.T) {
	kb := classroomKB(t)
	kb.TaskColumns.Construction[0].ReferenceIDs = []string{"does-not-exist"}
	if kb.validateTaskColumns() == nil {
		t.Fatal("broken table citation accepted")
	}
}

func TestSearchStreamsExplanationThenCompleteTable(t *testing.T) {
	kb := classroomKB(t)
	for _, mode := range []string{"brief", "deep", "research"} {
		t.Run(mode, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req chatRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				if len(req.Messages) != 2 || !strings.Contains(req.Messages[1].Content, "【本次任务表") {
					t.Error("model did not receive final table context")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				// Deliberately omit a table: the application must guarantee it.
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"# 自主创新\\n\\n先理解技术与优点。\"}}]}\n\ndata: [DONE]\n\n")
			}))
			defer upstream.Close()
			s := &Server{kb: kb, cfg: &Config{SiliconFlow: SiliconFlowConfig{APIKey: "test", Model: "test-model", BaseURL: upstream.URL}}}
			w := httptest.NewRecorder()
			s.handleSearch(w, httptest.NewRequest(http.MethodPost, "/api/search", strings.NewReader(fmt.Sprintf(`{"query":"舒适","mode":%q}`, mode))))
			var deltas []string
			var events []string
			for _, block := range strings.Split(w.Body.String(), "\n\n") {
				lines := strings.Split(block, "\n")
				if len(lines) < 2 {
					continue
				}
				event := strings.TrimPrefix(lines[0], "event: ")
				events = append(events, event)
				if event == "delta" {
					var data map[string]string
					if err := json.Unmarshal([]byte(strings.TrimPrefix(lines[1], "data: ")), &data); err != nil {
						t.Fatal(err)
					}
					deltas = append(deltas, data["t"])
				}
			}
			if !reflect.DeepEqual(events, []string{"meta", "references", "delta", "delta", "done"}) {
				t.Fatal("unexpected event order", events)
			}
			columns := kb.selectTaskColumns("舒适", mode)
			want := buildTaskTable(columns, kb.taskReferences("舒适", mode, columns), mode)
			if len(deltas) != 2 || deltas[1] != want || !strings.HasSuffix(strings.Join(deltas, ""), "|\n") {
				t.Fatal("answer did not end with complete verified table")
			}
		})
	}
}

func TestFailedGenerationDoesNotClaimTaskCompletion(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer upstream.Close()
	s := &Server{kb: classroomKB(t), cfg: &Config{SiliconFlow: SiliconFlowConfig{Model: "test", BaseURL: upstream.URL}}}
	w := httptest.NewRecorder()
	s.handleSearch(w, httptest.NewRequest(http.MethodPost, "/api/search", strings.NewReader(`{"query":"复兴号"}`)))
	if !strings.Contains(w.Body.String(), "event: error") || strings.Contains(w.Body.String(), "event: done") {
		t.Fatal("failure was presented as a complete answer")
	}
}
