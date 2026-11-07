//go:build !windows

package main

// decorateWindow — на других системах заголовок и значки окна рисует
// оконный менеджер по значку из SetIcon.
func decorateWindow(string) bool { return true }
