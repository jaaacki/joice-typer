#ifndef SETTINGS_DARWIN_H
#define SETTINGS_DARWIN_H

void showSetupWindow(void);
void updateSetupAccessibility(int granted);
void updateSetupInputMonitoring(int granted);
void populateSetupDevices(const char **deviceNames, int count, int defaultIndex);
void updateSetupDownloadProgress(double progress, long long bytesDownloaded, long long bytesTotal);
void updateSetupDownloadComplete(void);
void updateSetupDownloadFailed(const char *errorMsg);
void updateSetupReady(void);
int isSetupComplete(void);
const char *getSelectedDevice(void);
void runSetupEventLoop(void);

#endif
