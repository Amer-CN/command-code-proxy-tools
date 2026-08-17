#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""CommandCode Proxy Deck 双模式构建脚本（作者本地工具，不入公开仓库逻辑）。

用法:
    python build.py full    完整版（注入 .private：插件/授权/开发者模式 → CommandCodeProxyDeck.exe）
    python build.py public  公开版（纯净：无插件，仅供验证/分发到 GitHub 的源码对应产物）

完整版 = 公开源码 + .private 注入；.private 不存在时 full 会报错。
"""
import os
import shutil
import subprocess
import sys
import tempfile

ROOT = os.path.dirname(os.path.abspath(__file__))
PRIVATE = os.path.join(ROOT, ".private")

# 构建时需要注入的 .private 文件 → 目标路径（相对仓库根）
INJECT = [
    # Go 敏感包（整个目录）
    (os.path.join(PRIVATE, "go", "tuanjie"), "internal/tuanjie"),
    (os.path.join(PRIVATE, "go", "codebuddy"), "internal/codebuddy"),
    (os.path.join(PRIVATE, "go", "notion"), "internal/notion"),
    (os.path.join(PRIVATE, "go", "lingxi"), "internal/lingxi"),
    (os.path.join(PRIVATE, "go", "license"), "internal/license"),
    # app 包文件（同名覆盖公开 stub）
    (os.path.join(PRIVATE, "app", "plugin_modes.go"), "app/plugin_modes.go"),
    (os.path.join(PRIVATE, "app", "plugin_bindings.go"), "app/plugin_bindings.go"),
    (os.path.join(PRIVATE, "app", "plugins.go"), "app/plugins.go"),
    (os.path.join(PRIVATE, "app", "app.go"), "app/app.go"),
]

# UI 注入：占位符 → .private 内容
UI_DEV = os.path.join(PRIVATE, "app", "ui_dev.html")


def fail(msg):
    print("[构建失败]", msg, file=sys.stderr)
    sys.exit(1)


def copy_tree(src, dst):
    if os.path.isdir(src):
        shutil.copytree(src, dst, dirs_exist_ok=True)
    else:
        os.makedirs(os.path.dirname(dst), exist_ok=True)
        shutil.copy2(src, dst)


def build(mode):
    if mode not in ("full", "public"):
        fail("mode 参数须为 full 或 public")

    tmp = tempfile.mkdtemp(prefix="ccpd_build_")
    try:
        # 1) 复制仓库树（排除 .git/.private/bin/发布物）
        for name in os.listdir(ROOT):
            if name in (".git", ".private", "bin", "CommandCodeProxyDeck.exe",
                        "CommandCodeProxyDeck-Portable.zip", "__pycache__"):
                continue
            s = os.path.join(ROOT, name)
            copy_tree(s, os.path.join(tmp, name))

        # 2) 完整版：注入 .private
        if mode == "full":
            if not os.path.isdir(PRIVATE):
                fail(".private/ 不存在（完整版只能在作者本地构建）")
            for src, dst in INJECT:
                if not os.path.exists(src):
                    fail("缺少注入文件: %s" % src)
                copy_tree(src, os.path.join(tmp, dst))
            # UI：注入 ui_dev.html 的三个块（CSS / HTML / JS）
            ui_path = os.path.join(tmp, "app", "ui.html")
            ui = open(ui_path, encoding="utf-8").read()
            dev = open(UI_DEV, encoding="utf-8").read()
            def block(tag):
                start = dev.index(tag) + len(tag)
                end = dev.find("/*__", start)
                if end < 0:
                    end = len(dev)  # 最后一块取到文件尾
                return dev[start:end]
            ui = ui.replace("/*#DEV_CSS#*/", block("/*__DEV_CSS__*/"))
            ui = ui.replace("<!--#DEV_HTML#-->", block("/*__DEV_HTML__*/"))
            ui = ui.replace("//#DEV_JS#", block("/*__DEV_JS__*/"))
            open(ui_path, "w", encoding="utf-8", newline="").write(ui)
            print("[注入] .private → 完整版")

        # 3) go build（WebView2 CGO 需要 MinGW）。
        # 先构建到临时名，再原子替换——正在运行的 exe 会被 Windows 锁定，
        # 直接 -o 目标名会静默失败（go 把 a.out.exe 拷贝到目标时报错）。
        env = dict(os.environ)
        ldflags = "-H windowsgui -s -w"
        final = os.path.join(ROOT, "CommandCodeProxyDeck.exe" if mode == "full"
                             else "CommandCodeProxyDeck-public.exe")
        staging = os.path.join(ROOT, final + ".new")
        cmd = ["go", "build", "-trimpath", "-ldflags=" + ldflags, "-o", staging, "./app"]
        print("[构建]", " ".join(cmd), "(cwd=%s)" % tmp)
        r = subprocess.run(cmd, cwd=tmp, env=env)
        if r.returncode != 0:
            fail("go build 失败（exit %d）" % r.returncode)
        # 原子替换（目标被锁时 os.replace 失败并明确报错，不再静默）
        try:
            os.replace(staging, final)
        except PermissionError as e:
            os.remove(staging)
            fail("无法替换 %s（文件被运行中的进程锁定，请先关闭程序）: %s" % (final, e))

        size = os.path.getsize(final)
        print("[完成] %s 版 → %s (%.1f MB)" % (mode, final, size / 1048576))
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


if __name__ == "__main__":
    mode = sys.argv[1] if len(sys.argv) > 1 else "full"
    build(mode)
