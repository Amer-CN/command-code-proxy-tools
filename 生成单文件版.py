# -*- coding: utf-8 -*-
"""把 command-code-proxy.exe 以 base64 内嵌进 HTA 模板，生成单文件版控制台。
用法: python 生成单文件版.py
"""
import base64
import os

HERE = os.path.dirname(os.path.abspath(__file__))
EXE = os.path.join(HERE, "bin", "command-code-proxy.exe")
TEMPLATE = os.path.join(HERE, "hta_template.txt")
OUT = os.path.join(HERE, "CommandCode代理-单文件版.hta")
CHUNK = 100_000  # base64 字符分块，避免单行过长

def main():
    with open(EXE, "rb") as f:
        data = f.read()
    b64 = base64.b64encode(data).decode("ascii")
    chunks = [b64[i:i + CHUNK] for i in range(0, len(b64), CHUNK)]

    with open(TEMPLATE, "r", encoding="utf-8") as f:
        tpl = f.read()

    if "__BASE64__" not in tpl:
        raise SystemExit("模板中未找到 __BASE64__ 占位符")

    payload = 'var B64 = ""\n' + "".join(
        'B64 += "%s"\n' % c for c in chunks
    )
    out = tpl.replace("__BASE64__", payload)

    with open(OUT, "w", encoding="utf-8") as f:
        f.write(out)

    size_mb = os.path.getsize(OUT) / 1024 / 1024
    print("OK: %s (%.1f MB, %d 个分块, 原始 exe %d KB)" % (OUT, size_mb, len(chunks), len(data) // 1024))

if __name__ == "__main__":
    main()
