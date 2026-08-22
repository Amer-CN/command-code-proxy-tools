// plugin_modes.go —— 插件子模式：进程内直接运行对应插件服务（GUI 托管时 spawn 本模式）。
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/dev2k6/command-code-proxy-server/internal/codebuddy"
	"github.com/dev2k6/command-code-proxy-server/internal/lingxi"
	"github.com/dev2k6/command-code-proxy-server/internal/notion"
	"github.com/dev2k6/command-code-proxy-server/internal/tuanjie"
	"github.com/dev2k6/command-code-proxy-server/internal/zcoderemote"
)

var (
	flagPluginTuanjie     = flag.Bool("plugin-tuanjie", false, "团结 Cowork (Codely) 插件服务模式（GUI 托管时自动 spawn）")
	flagPluginCodebuddy   = flag.Bool("plugin-codebuddy", false, "CodeBuddy/WorkBuddy 插件服务模式（GUI 托管时自动 spawn；--desensitize 可选）")
	flagDesensitize       = flag.Bool("desensitize", false, "CodeBuddy 插件：对 system/developer/tools 做零宽脱敏，缓解腾讯审核误拦")
	flagPluginNotion      = flag.Bool("plugin-notion", false, "Notion AI 插件服务模式（凭据经 CDP 自动读取）")
	flagPluginLingxi      = flag.Bool("plugin-lingxi", false, "WPS 灵犀插件服务模式（凭据经 CDP 自动读取）")
	flagPluginZcodeRemote = flag.Bool("plugin-zcoderemote", false, "ZCode 多开额度插件服务模式（多账号 slot 聚合转发）")
)

// runPluginMode 处理 --plugin-tuanjie / --plugin-codebuddy / --plugin-notion / --plugin-lingxi / --plugin-zcoderemote
// 子模式：进程内直接跑对应插件服务（无窗口，关 GUI 不受影响）。
func runPluginMode() int {
	// 团结插件服务模式：进程内直接跑 internal/tuanjie 服务。
	if *flagPluginTuanjie {
		srv := tuanjie.NewServer()
		if err := srv.Start(*flagHost, *flagPort); err != nil {
			_ = os.WriteFile(filepath.Join(exeDir(), "tuanjie-plugin-error.log"),
				[]byte(err.Error()), 0o600)
			os.Exit(1)
		}
		select {}
	}

	// CodeBuddy 插件服务模式：读桌面端登录态直连腾讯后端。
	if *flagPluginCodebuddy {
		srv, err := codebuddy.NewServer(*flagDesensitize)
		if err != nil {
			_ = os.WriteFile(filepath.Join(exeDir(), "codebuddy-plugin-error.log"),
				[]byte(err.Error()), 0o600)
			os.Exit(1)
		}
		log.Printf("codebuddy-plugin: listening on %s:%s (backend copilot.tencent.com, desensitize=%v)",
			*flagHost, *flagPort, *flagDesensitize)
		if err := srv.Start(*flagHost, *flagPort); err != nil {
			_ = os.WriteFile(filepath.Join(exeDir(), "codebuddy-plugin-error.log"),
				[]byte(err.Error()), 0o600)
			os.Exit(1)
		}
		select {}
	}

	// Notion AI 插件服务模式：CDP 自动读桌面端令牌 → OpenAI 兼容端点。
	if *flagPluginNotion {
		srv := notion.NewServer()
		log.Printf("notion-plugin: starting on %s:%s", *flagHost, *flagPort)
		if err := srv.Start(*flagHost, *flagPort); err != nil {
			_ = os.WriteFile(filepath.Join(exeDir(), "notion-plugin-error.log"),
				[]byte(err.Error()), 0o600)
			os.Exit(1)
		}
		select {}
	}
	// WPS 灵犀插件服务模式。
	if *flagPluginLingxi {
		srv := lingxi.NewServer()
		log.Printf("lingxi-plugin: starting on %s:%s", *flagHost, *flagPort)
		if err := srv.Start(*flagHost, *flagPort); err != nil {
			_ = os.WriteFile(filepath.Join(exeDir(), "lingxi-plugin-error.log"), []byte(err.Error()), 0o600)
			os.Exit(1)
		}
		select {}
	}
	// ZCode 多开额度插件服务模式（多账号 slot 聚合 → 本地 OpenAI 兼容端点）。
	if *flagPluginZcodeRemote {
		srv := zcoderemote.NewServer()
		log.Printf("zcoderemote-plugin: starting on %s:%s", *flagHost, *flagPort)
		if err := srv.Start(*flagHost, *flagPort); err != nil {
			_ = os.WriteFile(filepath.Join(exeDir(), "zcoderemote-plugin-error.log"), []byte(err.Error()), 0o600)
			os.Exit(1)
		}
		select {}
	}
	return 0
}
