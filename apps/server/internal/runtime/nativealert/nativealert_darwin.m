// Objective-C half of the macOS fatal-alert bridge (docs/macos-packaging.md
// §12). Kept to the minimum needed to show one modal NSAlert - title and
// message are used strictly as string data, never evaluated, executed, or
// interpolated into any command of any kind.
#import <Cocoa/Cocoa.h>

void streamingTreeShowFatalAlert(const char *title, const char *message) {
    @autoreleasepool {
        // NSAlert needs a running NSApplication; a packaged app launched
        // normally already has one, but this may run before any other
        // AppKit call, so the shared instance is touched here to make
        // sure it exists.
        [NSApplication sharedApplication];

        NSAlert *alert = [[NSAlert alloc] init];
        alert.alertStyle = NSAlertStyleCritical;
        alert.messageText = [NSString stringWithUTF8String:title];
        alert.informativeText = [NSString stringWithUTF8String:message];
        [alert addButtonWithTitle:@"OK"];

        // The packaged app normally runs with no Dock icon
        // (LSUIElement, docs/macos-packaging.md §13); without this the
        // alert could appear behind other windows and look like nothing
        // happened.
        [NSApp activateIgnoringOtherApps:YES];
        [alert runModal];
    }
}
