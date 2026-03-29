#ifndef PASTER_DARWIN_H
#define PASTER_DARWIN_H

// pasteText saves the current clipboard, sets clipboard to text,
// simulates Cmd+V to paste, then restores the original clipboard
// after a 200ms delay.
// Returns 0 on success, non-zero on failure.
int pasteText(const char* text);

#endif
