//go:build windows

package windows

import (
	"fmt"
	"os/exec"
	"strings"
)

func PostNotification(title, body string) {
	if err := spawnWindowsToast(title, body); err != nil {
		currentSettingsLogger().Warn("failed to spawn windows toast", "operation", "PostNotification", "error", err)
	}
}

func escapeWindowsToastText(value string) string {
	value = strings.ReplaceAll(value, "`", "``")
	return strings.ReplaceAll(value, "'", "''")
}

func spawnWindowsToast(title, body string) error {
	script := fmt.Sprintf(`
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] > $null
$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml(@"
<toast>
  <visual>
    <binding template="ToastGeneric">
      <text>%s</text>
      <text>%s</text>
    </binding>
  </visual>
</toast>
"@)
$toast = [Windows.UI.Notifications.ToastNotification]::new($xml)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('JoiceTyper').Show($toast)
`, escapeWindowsToastText(title), escapeWindowsToastText(body))

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	return cmd.Start()
}
