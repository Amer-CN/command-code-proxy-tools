//go:build !private

// plugin_modes.go —— 公开版插件模式占位。
// 完整实现由作者本地 .private/app/plugin_modes.go 构建时注入；
// 公开版（无注入）收到插件子模式参数时提示后退出。
package main

import (
	"flag"
	"fmt"
	"os"
)

var (
	flagPluginTuanjie   = flag.Bool("plugin-tuanjie", false, "（本版本未包含）")
	flagPluginCodebuddy = flag.Bool("plugin-codebuddy", false, "（本版本未包含）")
	flagDesensitize     = flag.Bool("desensitize", false, "（本版本未包含）")
)

// runPluginMode 公开版：插件功能未包含在本次构建中。
func runPluginMode() int {
	fmt.Fprintln(os.Stderr, "此版本不包含插件服务模式。")
	return 1
}
