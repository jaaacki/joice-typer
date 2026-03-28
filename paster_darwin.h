#ifndef PASTER_DARWIN_H
#define PASTER_DARWIN_H

// setClipboard copies text to the macOS general pasteboard.
// Returns 0 on success, non-zero on failure.
int setClipboard(const char* text);

// simulateCmdV simulates pressing Cmd+V to paste from clipboard.
void simulateCmdV(void);

#endif
