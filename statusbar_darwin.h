#ifndef STATUSBAR_DARWIN_H
#define STATUSBAR_DARWIN_H

// initStatusBar creates the NSStatusItem with the Bubble J icon.
// Must be called from the main thread after NSApplication is initialized.
void initStatusBar(void);

// initStatusBarOnMainThread dispatches initStatusBar to the main queue
// and blocks until it completes. Safe to call from any thread.
void initStatusBarOnMainThread(void);

// updateStatusBar changes the icon color and menu text.
// state: 0=loading, 1=ready, 2=recording, 3=transcribing
void updateStatusBar(int state);

#endif
