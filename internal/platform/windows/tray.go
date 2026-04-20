//go:build windows

package windows

func InitStatusBar() {
}

func UpdateStatusBar(state AppState) {
	storeCurrentAppState(state)
	publishRuntimeStateChanged(state)
}

func SetStatusBarHotkeyText(string) {}
