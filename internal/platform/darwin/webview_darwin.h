void showWebSettingsWindow(const char *indexPath);
void dispatchWebSettingsScript(const char *script);
char *handleWebSettingsMessage(char *messageJSON);
void webSettingsWindowClosed(void);
void webSettingsNativeTransportWarning(char *operation, char *message);
void startWebHotkeyCapture(void);
void cancelWebHotkeyCapture(void);
int confirmWebHotkeyCapture(unsigned long long *flags, int *keycode);
