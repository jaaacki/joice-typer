//go:build windows

package windows

func InitStatusBar() {
}

func UpdateStatusBar(state AppState) {
	storeCurrentAppState(state)
}

func SetStatusBarHotkeyText(string) {}
