//go:build windows

package windows

func InitStatusBar() {
	storeCurrentAppState(StateLoading)
}

func InitStatusBarAsync() {
	InitStatusBar()
}

func UpdateStatusBar(state AppState) {
	storeCurrentAppState(state)
}

func SetStatusBarHotkeyText(string) {}
