const { test } = require('node:test');
const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const vm = require('node:vm');
const source = readFileSync(new URL('../static/js/app.js', `file://${__filename}`), 'utf8');

function app(searchFetch) {
  let state;
  const context = {
    Vue: {
      ref: value => ({ value }), reactive: value => value,
      computed: get => ({ get value() { return get(); } }),
      createApp: options => ({ use() { return this; }, mount() { state = options.setup(); } }),
    },
    ElementPlus: { ElMessage: { warning() {} } }, ElementPlusIconsVue: {},
    AbortController, TextDecoder, Date,
    fetch: (url, init) => url === '/api/kb'
      ? Promise.resolve({ json: async () => ({ count: 39, categories: [] }) })
      : searchFetch(url, init),
  };
  vm.runInNewContext(source, context);
  return state;
}
const event = (name, data) => `event: ${name}\ndata: ${JSON.stringify(data)}\n\n`;
function stream(text) {
  const bytes = new TextEncoder().encode(text);
  return new Response(new ReadableStream({
    start(controller) {
      // Include cuts inside Chinese UTF-8 characters and SSE event boundaries.
      for (let i = 0; i < bytes.length; i += 7) controller.enqueue(bytes.slice(i, i + 7));
      controller.close();
    },
  }));
}

test('streaming answer clears failed model text and renders final table after the explanation', async () => {
  const state = app(async () => stream(
    event('delta', { t: '失败模型的半截回答' }) + event('retry', { model: 'first' }) +
    event('delta', { t: '# 复兴号\n- 自主创新[1]\n' }) +
    event('delta', { t: '| 自主建造 | 自主研发 | 优点 |\n| --- | --- | --- |\n| 网络控制系统[1] | 车体 | 安全 |\n| 牵引传动系统 | 转向架 | 智能 |\n| 制动系统 | 低阻力外形 | 节能环保 |\n' }) +
    event('done', { model: 'second' })
  ));
  state.query.value = '复兴号';
  await state.doSearch();
  assert.equal(state.phase.value, 'done');
  assert.equal(state.usedModel.value, 'second');
  const html = state.renderedHTML.value;
  assert.ok(!html.includes('半截回答'));
  assert.match(html, /<\/ul><div class="table-scroll"/);
  assert.equal((html.match(/<td>/g) || []).length, 9);
  assert.match(html, /<span class="cite">1<\/span>/);
  assert.ok(html.endsWith('</table></div>'));
});

test('connection ending without done is not reported as a completed task', async () => {
  const state = app(async () => stream(event('delta', { t: '# 未完成' })));
  state.query.value = '复兴号';
  await state.doSearch();
  assert.equal(state.phase.value, 'error');
  assert.match(state.errorMsg.value, /连接中断/);
});

test('a stale response cannot overwrite a newer search', async () => {
  let release;
  let count = 0;
  const state = app(async () => {
    if (++count === 1) return new Promise(resolve => { release = resolve; });
    return stream(event('delta', { t: '# 新回答' }) + event('done', { model: 'new' }));
  });
  state.query.value = '旧问题';
  const old = state.doSearch();
  state.query.value = '新问题';
  await state.doSearch();
  release(stream(event('delta', { t: '# 旧回答' }) + event('done', { model: 'old' })));
  await old;
  assert.equal(state.usedModel.value, 'new');
  assert.match(state.renderedHTML.value, /新回答/);
  assert.ok(!state.renderedHTML.value.includes('旧回答'));
});

test('answer HTML is escaped before rendering', async () => {
  const state = app(async () => stream(event('delta', { t: '<img src=x onerror=alert(1)>' }) + event('done', {})));
  state.query.value = '复兴号';
  await state.doSearch();
  assert.ok(!state.renderedHTML.value.includes('<img'));
  assert.match(state.renderedHTML.value, /&lt;img/);
});
