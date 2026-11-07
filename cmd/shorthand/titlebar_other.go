//go:build !windows

package main

// darkTitleBar — на других системах заголовок окна рисует оконный менеджер.
func darkTitleBar(string) bool { return true }
