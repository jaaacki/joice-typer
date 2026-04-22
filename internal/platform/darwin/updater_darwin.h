#import <stdbool.h>

// startSparkleUpdater loads Sparkle.framework from the app bundle and starts
// the standard updater controller. Returns NULL on success or a heap-allocated
// error string that the Go caller must free.
char *startSparkleUpdater(void);

// checkForSparkleUpdates triggers a manual Sparkle update check using the
// already initialized updater controller. Returns NULL on success or a
// heap-allocated error string that the Go caller must free.
char *checkForSparkleUpdates(void);
