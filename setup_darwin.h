#ifndef SETUP_DARWIN_H
#define SETUP_DARWIN_H

void showSetupWindow(void);
void updateSetupAccessibility(int granted);
void populateSetupDevices(const char **deviceNames, int count, int defaultIndex);
void updateSetupDownloadProgress(double progress, long long bytesDownloaded, long long bytesTotal);
void updateSetupDownloadComplete(void);
void updateSetupReady(void);
int isSetupComplete(void);
const char *getSelectedDevice(void);

#endif
