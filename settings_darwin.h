#ifndef SETTINGS_DARWIN_H
#define SETTINGS_DARWIN_H

#include <stdint.h>

void showSettingsWindow(int onboarding);
void updateSetupAccessibility(int granted);
void updateSetupInputMonitoring(int granted);
void populateSetupDevices(const char **deviceNames, int count, int defaultIndex);
void populateSettingsLanguages(const char **codes, const char **names, int count, int defaultIndex);
const char *getSelectedLanguage(void);
void populateSettingsModels(const char **sizes, const char **descriptions, int count, int defaultIndex);
const char *getSelectedModel(void);
void updateSettingsModelStatus(const char *status);
void setSettingsHotkey(const char *displayText);
const char *getSettingsHotkey(void);
uint64_t getSettingsHotkeyFlags(void);
int getSettingsHotkeyKeycode(void);
void updateSetupDownloadProgress(double progress, long long bytesDownloaded, long long bytesTotal);
void updateSetupDownloadComplete(void);
void updateSetupDownloadFailed(const char *errorMsg);
void updateSetupReady(void);
void setPrefsPermissionState(void);
int isSetupComplete(void);
const char *getSelectedDevice(void);
void runSetupEventLoop(void);

#endif
