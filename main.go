// mita-search: 模拟秘塔AI搜索（复兴号知识库版）
// Go 后端：静态文件服务 + 知识库检索 + SiliconFlow 流式 RAG 问答（SSE）
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------- 配置 ----------

type SiliconFlowConfig struct {
	APIKey         string   `json:"api_key"`
	BaseURL        string   `json:"base_url"`
	Model          string   `json:"model"`
	FallbackModels []string `json:"fallback_models"`
	MaxTokens      int      `json:"max_tokens"`
	Temperature    float64  `json:"temperature"`
}

type Config struct {
	ListenPort  int               `json:"listen_port"`
	SiliconFlow SiliconFlowConfig `json:"siliconflow"`
}

func loadConfig(path string) (*Config, error) {
	cfg := &Config{
		ListenPort: 8066,
		SiliconFlow: SiliconFlowConfig{
			BaseURL:        "https://api.siliconflow.cn/v1/chat/completions",
			Model:          "Qwen/Qwen3.5-27B",
			FallbackModels: []string{"Qwen/Qwen2.5-72B-Instruct", "Qwen/Qwen2.5-7B-Instruct"},
			MaxTokens:      3000,
			Temperature:    0.5,
		},
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	}
	if port := os.Getenv("LISTEN_PORT"); port != "" {
		cfg.ListenPort, err = strconv.Atoi(port)
		if err != nil {
			return nil, fmt.Errorf("LISTEN_PORT 必须是整数")
		}
	}
	if cfg.ListenPort < 1 || cfg.ListenPort > 65535 {
		return nil, fmt.Errorf("listen_port 必须在 1 到 65535 之间")
	}
	for name, target := range map[string]*string{
		"SILICONFLOW_API_KEY":  &cfg.SiliconFlow.APIKey,
		"SILICONFLOW_BASE_URL": &cfg.SiliconFlow.BaseURL,
		"SILICONFLOW_MODEL":    &cfg.SiliconFlow.Model,
	} {
		if value := os.Getenv(name); value != "" {
			*target = value
		}
	}
	if value, ok := os.LookupEnv("SILICONFLOW_FALLBACK_MODELS"); ok {
		cfg.SiliconFlow.FallbackModels = nil
		for _, model := range strings.Split(value, ",") {
			if model = strings.TrimSpace(model); model != "" {
				cfg.SiliconFlow.FallbackModels = append(cfg.SiliconFlow.FallbackModels, model)
			}
		}
	}
	if strings.TrimSpace(cfg.SiliconFlow.APIKey) == "" {
		return nil, fmt.Errorf("请设置 SILICONFLOW_API_KEY 或 config.json 中的 siliconflow.api_key")
	}
	return cfg, nil
}

// ---------- 知识库 ----------

type KnowledgeItem struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Category string   `json:"category"`
	Content  string   `json:"content"`
	Keywords []string `json:"keywords"`
	Source   string   `json:"source"`
	URL      string   `json:"url"`
}

type KnowledgeBase struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Focus       string          `json:"focus"`
	Items       []KnowledgeItem `json:"items"`
}

func loadKnowledge(path string) (*KnowledgeBase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	kb := &KnowledgeBase{}
	if err := json.Unmarshal(data, kb); err != nil {
		return nil, err
	}
	return kb, nil
}

type ScoredItem struct {
	Item  KnowledgeItem `json:"item"`
	Score float64       `json:"score"`
}

// bigrams 把字符串按字符切成二元组（中文检索用）
func bigrams(s string) []string {
	r := []rune(strings.ToLower(s))
	var gs []string
	for i := 0; i+1 < len(r); i++ {
		gs = append(gs, string(r[i:i+2]))
	}
	if len(r) == 1 {
		gs = append(gs, string(r[0]))
	}
	return gs
}

// Search 简单加权检索：标题/关键词命中权重高，正文命中权重低
func (kb *KnowledgeBase) Search(query string, topK int) []ScoredItem {
	q := strings.TrimSpace(strings.ToLower(query))
	if q == "" {
		return nil
	}
	grams := bigrams(q)
	var scored []ScoredItem

	for _, item := range kb.Items {
		score := 0.0
		title := strings.ToLower(item.Title)
		content := strings.ToLower(item.Content)

		// 标题/关键词整词命中
		if strings.Contains(title, q) || strings.Contains(q, title) {
			score += 12
		}
		for _, kw := range item.Keywords {
			kl := strings.ToLower(kw)
			if kl == "" {
				continue
			}
			if strings.Contains(q, kl) {
				score += 8
			} else if len([]rune(q)) >= 2 && strings.Contains(kl, q) {
				score += 4
			}
		}
		// 二元组模糊命中
		for _, g := range grams {
			if len([]rune(g)) < 2 {
				continue
			}
			if strings.Contains(title, g) {
				score += 2.0
			}
			for _, kw := range item.Keywords {
				if strings.Contains(strings.ToLower(kw), g) {
					score += 1.2
				}
			}
			if strings.Contains(content, g) {
				score += 0.4
			}
		}
		if score > 1.0 {
			scored = append(scored, ScoredItem{Item: item, Score: score})
		}
	}

	sort.Slice(scored, func(i, j int) bool { return scored[i].Score > scored[j].Score })
	if len(scored) > topK {
		scored = scored[:topK]
	}
	return scored
}

// ---------- 搜索请求 ----------

type SearchRequest struct {
	Query string `json:"query"`
	Mode  string `json:"mode"` // brief / deep / research
}

func topKForMode(mode string) int {
	switch mode {
	case "brief":
		return 4
	case "research":
		return 10
	default: // deep
		return 7
	}
}

// ---------- SSE 辅助 ----------

func writeSSE(w http.ResponseWriter, event string, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(data))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// ---------- 系统提示词 ----------

func buildSystemPrompt(mode string) string {
	base := `你是"高小铁"，一个面向小学课堂的AI搜索助手，只基于给定知识库资料回答问题。请严格遵守以下规则：
1. 只依据【参考资料】回答，禁止编造资料之外的内容、数字或事实。
2. 用中文回答，输出Markdown格式：
   - 第一行用"# "写一个简明的主题标题；
   - 然后用"## 小标题"组织板块；板块内容用简洁的列表（- ）或短段落阐述；
   - 如果用户要求填写表格或对比内容，用Markdown表格（| 列 | 列 |）输出。
3. 每当某句话的信息来自某条资料，请在句末或列表项末尾标注来源编号，如[1]、[2]，编号与参考资料一致。
4. 语言准确、条理清晰，面向小学高年级学生和教师，深入浅出，不堆砌术语。
5. 不要出现"根据参考资料"之类的话，直接陈述答案；不要写总结性的废话或客套语，答完即止，以节约算力。`
	switch mode {
	case "brief":
		base += "\n6. 本次为【简洁】模式：全文严格控制在250字以内，1~2个小板块，直击要点。"
	case "research":
		base += "\n6. 本次为【研究】模式：全文控制在900字以内，4~5个小板块，充分利用资料但禁止注水。"
	default:
		base += "\n6. 本次为【深入】模式：全文控制在550字以内，2~4个小板块。"
	}
	return base
}

// maxTokensForMode 按模式限制生成 token，避免浪费（中文 1 token ≈ 0.7 字）
func maxTokensForMode(mode string) int {
	switch mode {
	case "brief":
		return 400
	case "research":
		return 1400
	default:
		return 850
	}
}

func buildReferencesBlock(refs []ScoredItem) string {
	var b strings.Builder
	b.WriteString("【参考资料】\n")
	for i, r := range refs {
		b.WriteString(fmt.Sprintf("[%d] %s（分类：%s）\n%s\n\n", i+1, r.Item.Title, r.Item.Category, r.Item.Content))
	}
	return b.String()
}

// ---------- SiliconFlow 流式调用 ----------

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature float64       `json:"temperature"`
	// EnableThinking Qwen3 系思考模型专用：false 关闭思考链，直接输出正文
	EnableThinking *bool `json:"enable_thinking,omitempty"`
}

// streamLLM 调用 SiliconFlow，把每个增量文本写入 onDelta；出错返回 error。
// 含首字看门狗：firstTokenTimeout 内没有任何正文输出则中断（自动换下一个模型）。
func streamLLM(cfg *SiliconFlowConfig, model string, messages []chatMessage, maxTokens int, firstTokenTimeout time.Duration, onDelta func(string)) error {
	req := chatRequest{
		Model:       model,
		Messages:    messages,
		Stream:      true,
		MaxTokens:   maxTokens,
		Temperature: cfg.Temperature,
	}
	// Qwen3 系思考模型：默认关闭思考链，节省 token 与等待时间
	if strings.Contains(model, "Qwen3") {
		no := false
		req.EnableThinking = &no
	}
	reqBody, _ := json.Marshal(req)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, "POST", cfg.BaseURL, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	// 首字看门狗：超时未输出正文则取消本次请求
	var once sync.Once
	firstDelta := make(chan struct{})
	watchdog := time.AfterFunc(firstTokenTimeout, func() {
		select {
		case <-firstDelta:
		default:
			cancel()
		}
	})
	defer watchdog.Stop()

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return fmt.Errorf("首字超时（%v 内未输出正文）", firstTokenTimeout)
		}
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("siliconflow %s 返回 %d: %s", model, resp.StatusCode, string(body))
	}

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line != "" {
			if strings.HasPrefix(line, "data:") {
				data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if data == "[DONE]" {
					return nil
				}
				var chunk struct {
					Choices []struct {
						Delta struct {
							Content string `json:"content"`
						} `json:"delta"`
					} `json:"choices"`
				}
				if json.Unmarshal([]byte(data), &chunk) == nil {
					for _, c := range chunk.Choices {
						if c.Delta.Content != "" {
							once.Do(func() { close(firstDelta) })
							onDelta(c.Delta.Content)
						}
					}
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// ---------- HTTP 处理器 ----------

type Server struct {
	cfg *Config
	kb  *KnowledgeBase
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req SearchRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		http.Error(w, "empty query", http.StatusBadRequest)
		return
	}

	// 1. 检索知识库
	refs := s.kb.Search(req.Query, topKForMode(req.Mode))

	// 2. 建立 SSE 流
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	writeSSE(w, "meta", map[string]interface{}{
		"query": req.Query,
		"mode":  req.Mode,
		"kb":    s.kb.Name,
	})
	writeSSE(w, "references", refs)

	// 3. 构建消息并流式生成
	messages := []chatMessage{
		{Role: "system", Content: buildSystemPrompt(req.Mode)},
		{Role: "user", Content: buildReferencesBlock(refs) + "【用户问题】\n" + req.Query},
	}

	var buf bytes.Buffer
	flushDelta := func(t string) {
		buf.WriteString(t)
		writeSSE(w, "delta", map[string]string{"t": t})
	}

	models := []string{s.cfg.SiliconFlow.Model}
	models = append(models, s.cfg.SiliconFlow.FallbackModels...)

	var lastErr error
	started := false
	maxTokens := maxTokensForMode(req.Mode)
	for _, m := range models {
		buf.Reset()
		started = false
		err := streamLLM(&s.cfg.SiliconFlow, m, messages, maxTokens, 45*time.Second, func(t string) {
			if !started {
				started = true
			}
			flushDelta(t)
		})
		if err == nil && buf.Len() > 0 {
			writeSSE(w, "done", map[string]interface{}{
				"model":  m,
				"length": buf.Len(),
			})
			return
		}
		lastErr = err
		// 该模型失败，清掉可能已输出的半截内容并尝试下一个
		writeSSE(w, "retry", map[string]string{"model": m})
		buf.Reset()
		if err == nil && buf.Len() == 0 {
			lastErr = fmt.Errorf("模型 %s 返回空内容", m)
		}
	}

	log.Printf("search failed: query=%q err=%v", req.Query, lastErr)
	writeSSE(w, "error", map[string]string{
		"message": "AI 服务暂时不可用，请稍后重试或检查 SiliconFlow API 配置。",
	})
}

func (s *Server) handleKB(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"name":        s.kb.Name,
		"description": s.kb.Description,
		"focus":       s.kb.Focus,
		"count":       len(s.kb.Items),
		"categories":  listCategories(s.kb),
	})
}

func listCategories(kb *KnowledgeBase) []string {
	seen := map[string]bool{}
	var cats []string
	for _, it := range kb.Items {
		if !seen[it.Category] {
			seen[it.Category] = true
			cats = append(cats, it.Category)
		}
	}
	return cats
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "time": time.Now().Format(time.RFC3339)})
}

func main() {
	dir, _ := os.Getwd()

	cfg, err := loadConfig(filepath.Join(dir, "config.json"))
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	kb, err := loadKnowledge(filepath.Join(dir, "knowledge", "knowledge_base.json"))
	if err != nil {
		log.Fatalf("加载知识库失败: %v", err)
	}

	srv := &Server{cfg: cfg, kb: kb}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/search", srv.handleSearch)
	mux.HandleFunc("/api/kb", srv.handleKB)
	mux.HandleFunc("/api/health", srv.handleHealth)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// 静态文件服务（static 目录）
		staticDir := filepath.Join(dir, "static")
		path := filepath.Clean(r.URL.Path)
		if path == "/" || path == "." {
			path = "/index.html"
		}
		full := filepath.Join(staticDir, path)
		if !strings.HasPrefix(full, filepath.Clean(staticDir)+string(os.PathSeparator)) {
			http.NotFound(w, r)
			return
		}
		if _, err := os.Stat(full); err != nil {
			// 前端单页回退
			http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
			return
		}
		http.ServeFile(w, r, full)
	})

	addr := fmt.Sprintf(":%d", cfg.ListenPort)
	log.Printf("✅ 秘塔AI搜索（复兴号知识库版）已启动: http://localhost:%d", cfg.ListenPort)
	log.Printf("   知识库: %s（%d 条）| 模型: %s", kb.Name, len(kb.Items), cfg.SiliconFlow.Model)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
