#!/usr/bin/env python3
"""高小铁 AI 助手 —— 50 台学生机并发压测脚本
模拟 50 个学生同时提问，统计成功率、首字延迟、总耗时、错误类型。
用法: python3 stress_test.py [并发数] (默认50)
"""
import json
import sys
import threading
import time
import urllib.request
from collections import Counter

BASE = "http://localhost:8066"
N = int(sys.argv[1]) if len(sys.argv) > 1 else 50

QUERIES = [
    ("复兴号有哪些自主创新？", "deep"),
    ("帮我完成课本做一做表格：复兴号的自主创新及优点", "deep"),
    ("复兴号的中国脑指的是什么？", "brief"),
    ("复兴号的速度有多快？", "brief"),
    ("为什么要研制复兴号？", "deep"),
    ("复兴号有哪些优点？", "brief"),
    ("复兴号的牵引传动系统有多强？", "deep"),
    ("复兴号是什么时候实现31个省区市全覆盖的？", "brief"),
    ("复兴号的安全保障技术有哪些？", "deep"),
    ("复兴号的意义是什么？", "deep"),
]

proxy_less = urllib.request.build_opener(urllib.request.ProxyHandler({}))

results = []
lock = threading.Lock()


def worker(i):
    q, mode = QUERIES[i % len(QUERIES)]
    body = json.dumps({"query": q, "mode": mode}).encode()
    req = urllib.request.Request(
        BASE + "/api/search", data=body,
        headers={"Content-Type": "application/json"}, method="POST")
    t0 = time.time()
    rec = {"id": i, "query": q, "mode": mode}
    try:
        resp = proxy_less.open(req, timeout=180)
        if resp.status != 200:
            rec.update(status="HTTP%d" % resp.status, ttfb=0, total=time.time() - t0, chars=0)
        else:
            first = None
            chars = 0
            model = ""
            err = ""
            while True:
                line = resp.readline()
                if not line:
                    break
                line = line.decode("utf-8", "ignore").strip()
                if line.startswith("data:"):
                    try:
                        d = json.loads(line[5:].strip())
                    except Exception:
                        continue
                    if "t" in d:
                        if first is None:
                            first = time.time() - t0
                        chars += len(d["t"])
                    elif "model" in d:
                        model = d["model"]
                    elif "message" in d:
                        err = d["message"]
            rec.update(
                status="error" if (err or chars == 0) else "ok",
                err=err, model=model, chars=chars,
                ttfb=round(first or 0, 2), total=round(time.time() - t0, 2))
    except Exception as e:
        rec.update(status="exc:%s" % type(e).__name__, err=str(e)[:80],
                   ttfb=0, total=round(time.time() - t0, 2), chars=0)
    with lock:
        results.append(rec)


print(f"🚄 高小铁压测：{N} 台学生机同时提问 ...")
t_start = time.time()
threads = [threading.Thread(target=worker, args=(i,)) for i in range(N)]
for t in threads:
    t.start()
for t in threads:
    t.join()
wall = time.time() - t_start

ok = [r for r in results if r["status"] == "ok"]
fail = [r for r in results if r["status"] != "ok"]
ttfbs = sorted(r["ttfb"] for r in ok if r["ttfb"])
totals = sorted(r["total"] for r in ok)
chars_list = [r["chars"] for r in ok]
models = Counter(r.get("model", "?") for r in ok)
fails = Counter(r["status"] for r in fail)

print(f"\n========== 压测报告（{N} 并发） ==========")
print(f"总耗时: {wall:.1f}s | 成功: {len(ok)} | 失败: {len(fail)}")
if ok:
    print(f"首字延迟(TTFB): 平均 {sum(ttfbs)/len(ttfbs):.1f}s | P50 {ttfbs[len(ttfbs)//2]:.1f}s | 最大 {ttfbs[-1]:.1f}s")
    print(f"完成耗时: 平均 {sum(totals)/len(totals):.1f}s | P50 {totals[len(totals)//2]:.1f}s | 最大 {totals[-1]:.1f}s")
    print(f"回答字数: 平均 {sum(chars_list)/len(chars_list):.0f} 字 | 最小 {min(chars_list)} | 最大 {max(chars_list)}")
    print(f"模型分布: {dict(models)}")
    print(f"吞吐: {len(ok)/wall:.1f} 请求/秒（整批 {wall:.0f}s 内完成）")
if fail:
    print(f"失败类型: {dict(fails)}")
    for r in fail[:5]:
        print(f"  - #{r['id']} {r['status']}: {r.get('err','')[:70]}")
