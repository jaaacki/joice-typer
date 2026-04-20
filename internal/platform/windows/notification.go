//go:build windows

package windows

func PostNotification(title, body string) {
	go func() {
		if err := showWindowsTrayNotification(title, body); err != nil {
			currentSettingsLogger().Warn("failed to show windows notification", "operation", "PostNotification", "error", err)
		}
	}()
}
