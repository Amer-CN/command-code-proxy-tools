//go:build !private

// plugin_bindings.go —— 公开版：插件/开发者模式绑定为空。
// 完整版由作者本地 .private/app/plugin_bindings.go 构建时注入。
package main

import (
	webview "github.com/webview/webview_go"
)

// bindPluginBindings 公开版不注册任何插件相关绑定。
func (a *app) bindPluginBindings(w webview.WebView) {}
