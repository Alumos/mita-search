/* 秘塔AI搜索（复兴号知识库版） 前端逻辑 */
const { createApp, ref, reactive, computed } = Vue;
const { ElMessage } = ElementPlus;
const {
  Search, Compass, Collection, Loading, CircleCheck,
  ChatDotRound, Files, Document,
} = ElementPlusIconsVue;

createApp({
  setup() {
    // ---------- 状态 ----------
    const stage = ref('home');           // home | result
    const query = ref('');
    const mode = ref('deep');
    const homeFocus = ref(false);

    const phase = ref('idle');           // searching | generating | done | error
    const references = ref([]);
    const rawMD = ref('');
    const usedModel = ref('');
    const errorMsg = ref('');
    const elapsed = ref(0);
    const openRef = ref(-1);

    const kbInfo = reactive({ count: 0, categories: [] });

    const hotQuestions = [
      '复兴号 关键核心技术 自主创新 阶段性成果',
      '复兴号 智能高铁 自动驾驶',
      '2018 京沈高铁 自动驾驶试验',
      '复兴号 中国脑 中国心',
      '复兴号 自主研发 安全 智能',
      '复兴号 科技创新 自主可控',
    ];
    const allNextAsks = [
      '复兴号 网络控制系统 自主研发',
      '复兴号 牵引传动系统 核心技术',
      '京沈高铁 试验 京张高铁 自动驾驶应用',
      '复兴号 车体 转向架 技术创新',
      '复兴号 故障预测 健康管理',
      '复兴号 地震预警 安全防护',
      '复兴号 节能环保 减振降噪',
      '复兴号 智能运维 远程监测',
      '复兴号 仿真测试 软件开发平台',
      '复兴号 中国标准 互联互通',
      '复兴号 高原应用 定制化创新',
      '复兴号 关键核心技术 自主可控 意义',
    ];

    let nextAsks = ref([]);
    let abortCtrl = null;

    // ---------- 知识库信息 ----------
    fetch('/api/kb').then(r => r.json()).then(d => {
      kbInfo.count = d.count;
      kbInfo.categories = d.categories;
    }).catch(() => {});

    // ---------- Markdown 渲染 ----------
    function escapeHtml(s) {
      return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
    }
    function inline(s) {
      // 先转义
      let t = escapeHtml(s);
      // 粗体
      t = t.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
      // 行内代码
      t = t.replace(/`([^`]+)`/g, '<code style="background:#f2f3f8;padding:1px 5px;border-radius:4px;font-size:13px">$1</code>');
      // 引用标记 [1] / [1][2] / [1,2]
      t = t.replace(/\[(\d+(?:[,，、]\s*\d+)*(?:\]\[\d+)*)\]/g, (m) => {
        const nums = m.match(/\d+/g) || [];
        return nums.map(n => `<span class="cite">${n}</span>`).join('');
      });
      return t;
    }

    function renderMarkdown(md) {
      const lines = md.split('\n');
      let html = '', i = 0;
      let listMode = null; // 'ul' | 'ol'

      const closeList = () => { if (listMode) { html += `</${listMode}>`; listMode = null; } };

      while (i < lines.length) {
        const line = lines[i];
        const trimmed = line.trim();

        if (trimmed === '') { closeList(); i++; continue; }

        // 表格
        if (/^\|.*\|/.test(trimmed)) {
          closeList();
          const rows = [];
          while (i < lines.length && /^\s*\|.*\|/.test(lines[i])) {
            rows.push(lines[i].trim());
            i++;
          }
          if (rows.length >= 2 && /^\|[\s:\-|]+\|$/.test(rows[1])) {
            const parseRow = (r) => r.replace(/^\|/, '').replace(/\|$/, '').split('|').map(c => c.trim());
            const head = parseRow(rows[0]);
            const body = rows.slice(2).map(parseRow);
            html += '<div class="table-scroll" tabindex="0" role="region" aria-label="创新成果表，可横向滚动"><table><thead><tr>' + head.map(h => `<th>${inline(h)}</th>`).join('') + '</tr></thead><tbody>';
            for (const r of body) html += '<tr>' + r.map(c => `<td>${inline(c)}</td>`).join('') + '</tr>';
            html += '</tbody></table></div>';
          } else {
            html += rows.map(row => '<p>' + inline(row) + '</p>').join('');
          }
          continue;
        }

        // 标题
        const h = trimmed.match(/^(#{1,4})\s+(.*)$/);
        if (h) {
          closeList();
          const level = h[1].length;
          html += `<h${level}>${inline(h[2])}</h${level}>`;
          i++; continue;
        }

        // 引用块
        if (trimmed.startsWith('>')) {
          closeList();
          let quote = [];
          while (i < lines.length && lines[i].trim().startsWith('>')) {
            quote.push(lines[i].trim().replace(/^>\s?/, ''));
            i++;
          }
          html += `<blockquote>${inline(quote.join(' '))}</blockquote>`;
          continue;
        }

        // 无序列表
        const ul = trimmed.match(/^[-*]\s+(.*)$/);
        if (ul) {
          if (listMode !== 'ul') { closeList(); html += '<ul>'; listMode = 'ul'; }
          html += `<li>${inline(ul[1])}</li>`;
          i++; continue;
        }

        // 有序列表
        const ol = trimmed.match(/^\d+[.、]\s+(.*)$/);
        if (ol) {
          if (listMode !== 'ol') { closeList(); html += '<ol>'; listMode = 'ol'; }
          html += `<li>${inline(ol[1])}</li>`;
          i++; continue;
        }

        // 分隔线
        if (/^(-{3,}|\*{3,})$/.test(trimmed)) { closeList(); html += '<hr style="border:none;border-top:1px solid #e5e6eb;margin:14px 0"/>'; i++; continue; }

        // 普通段落
        closeList();
        html += `<p>${inline(trimmed)}</p>`;
        i++;
      }
      closeList();
      return html;
    }

    const renderedHTML = computed(() => renderMarkdown(rawMD.value));

    // ---------- SSE 搜索 ----------
    function pickNextAsks() {
      const pool = allNextAsks.filter(q => q !== query.value);
      nextAsks.value = pool.sort(() => Math.random() - 0.5).slice(0, 3);
    }

    async function doSearch() {
      const q = query.value.trim();
      if (!q) { ElMessage.warning('请输入想搜索的内容'); return; }
      if (abortCtrl) abortCtrl.abort();
      const requestCtrl = new AbortController();
      abortCtrl = requestCtrl;

      stage.value = 'result';
      phase.value = 'searching';
      references.value = [];
      rawMD.value = '';
      usedModel.value = '';
      errorMsg.value = '';
      openRef.value = -1;
      const t0 = Date.now();
      let completed = false;

      try {
        const resp = await fetch('/api/search', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ query: q, mode: mode.value }),
          signal: requestCtrl.signal,
        });
        if (abortCtrl !== requestCtrl) return;
        if (!resp.ok) throw new Error('服务返回 ' + resp.status);

        phase.value = 'generating';
        const reader = resp.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';

        const handleEvent = (event, data) => {
          if (abortCtrl !== requestCtrl) return;
          if (event === 'references') {
            references.value = data || [];
          } else if (event === 'delta') {
            rawMD.value += data.t || '';
          } else if (event === 'answer') {
            rawMD.value = data.t || '';
          } else if (event === 'done') {
            usedModel.value = data.model || '';
            completed = true;
          } else if (event === 'retry') {
            rawMD.value = '';
            usedModel.value = '';
            completed = false;
          } else if (event === 'error') {
            errorMsg.value = data.message || 'AI 服务出错';
            phase.value = 'error';
          }
        };

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });
          let idx;
          while ((idx = buffer.indexOf('\n\n')) >= 0) {
            const raw = buffer.slice(0, idx);
            buffer = buffer.slice(idx + 2);
            let event = 'message', dataStr = '';
            for (const line of raw.split('\n')) {
              if (line.startsWith('event:')) event = line.slice(6).trim();
              else if (line.startsWith('data:')) dataStr += line.slice(5).trim();
            }
            if (dataStr) {
              try { handleEvent(event, JSON.parse(dataStr)); } catch (e) {}
            }
          }
        }

        if (abortCtrl !== requestCtrl) return;
        if (phase.value !== 'error' && !completed) throw new Error('回答连接中断，请重新搜索');
        if (phase.value !== 'error') {
          phase.value = 'done';
          elapsed.value = ((Date.now() - t0) / 1000).toFixed(1);
          pickNextAsks();
        }
      } catch (err) {
        if (err.name === 'AbortError' || abortCtrl !== requestCtrl) return;
        errorMsg.value = '网络错误：' + err.message;
        phase.value = 'error';
      }
    }

    function toggleRef(i) { openRef.value = openRef.value === i ? -1 : i; }
    function backHome() {
      if (abortCtrl) abortCtrl.abort();
      abortCtrl = null;
      stage.value = 'home';
      phase.value = 'idle';
    }

    return {
      stage, query, mode, homeFocus,
      phase, references, renderedHTML, usedModel, errorMsg, elapsed, openRef,
      kbInfo, hotQuestions, nextAsks,
      doSearch, toggleRef, backHome,
      Search, Compass, Collection, Loading, CircleCheck, ChatDotRound, Files, Document,
    };
  },
}).use(ElementPlus).mount('#app');
