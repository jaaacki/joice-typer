#ifndef SETTINGS_DARWIN_H
#define SETTINGS_DARWIN_H

#include <stdint.h>

void showSettingsWindow(int onboarding);
void openAccessibilitySettingsFromGo(void);
void openInputMonitoringSettingsFromGo(void);
void updateSetupAccessibility(int granted);
void updateSetupInputMonitoring(int granted);
void populateSetupDevices(const char **deviceNames, int count, int defaultIndex);
void populateSettingsLanguages(const char **codes, const char **names, int count, int defaultIndex);
const char *getSelectedLanguage(void);
void populateSettingsDecodeModes(const char **codes, const char **names, int count, int defaultIndex);
const char *getSelectedDecodeMode(void);
void populateSettingsPunctuationModes(const char **codes, const char **names, int count, int defaultIndex);
const char *getSelectedPunctuationMode(void);
void populateSettingsModels(const char **sizes, const char **descriptions, int count, int defaultIndex);
const char *getSelectedModel(void);
const char *getDropdownModel(void);
void setActiveModelSize(const char *size);
void updateModelButtons(int state);
void updateDownloadProgress(double progress, long long downloaded, long long total);
void setVocabularyText(const char *text);
const char *getVocabularyText(void);
void setSettingsHotkey(const char *displayText);
const char *getSettingsHotkey(void);
uint64_t getSettingsHotkeyFlags(void);
int getSettingsHotkeyKeycode(void);
void updateSetupDownloadComplete(void);
void updateSetupReady(void);
void setPrefsPermissionState(void);
int isSetupComplete(void);
const char *getSelectedDevice(void);
void runSetupEventLoop(void);
int copyTextToClipboard(const char *text);
int getLoginItemEnabled(void);
int setLoginItemEnabled(int enabled);

// Input device volume (0.0-1.0). Returns -1.0 if the device does not
// support software volume control. Pass "" for default input device.
float getInputDeviceVolume(const char *deviceName);
// Returns 1 on success, 0 on failure.
int setInputDeviceVolume(const char *deviceName, float volume);

// Microphone mode (Voice Isolation). macOS 12+ via AVCaptureDevice.
// Returns: -1 unavailable, 0 standard, 1 wide-spectrum, 2 voice-isolation.
int getPreferredMicrophoneMode(void);
int getActiveMicrophoneMode(void);
int setPreferredMicrophoneMode(int mode);

#endif
