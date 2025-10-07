# GO Development Setup Guide

This guide assumes you do not already have other C compilers installed in the system that will interfere with the setup below.

!! LLVM 21.x has a bug that generates invalid object files, the last known working version is 20.1.8. The bug will be fixed once llvm 21.1.3 is released !!

1. Install Go from https://go.dev/doc/install (I installed in C:\Go; any path is fine but adjust steps below accordingly)

2. Download llvm-mingw from https://github.com/mstorsjo/llvm-mingw/releases (As of writing this doc, I used llvm-mingw-20250709-ucrt-x86_64.zip
 on Windows 11 25H2)
3. Unzip to C:\ (Any path is fine but adjust steps below accordingly)
   - You will get a folder called "llvm-mingw-20250709-ucrt-x86_64" inside the destination folder/drive
   - Rename that folder to "llvm-mingw" for simplicity

4. Open "Environment Variables" from the System Properties dialog.
   Right-click "This PC" on your Desktop and click "Properties" to launch the System Properties dialog.
   Or use keyboard shortcut - WinKey + Pause-Break, then press "Advanced system settings" about 1/3rd down to the right

5. Under "User variables" find "Path", select it and click "Edit"
6. Make sure "%USERPROFILE%\go\bin" is in the Path list. If it's not, add it by pressing "New"
   - This is the folder where "go install" will create binaries, so we want it as a Path entry to be able to execute the files globally

7. Under "System variables" find "Path", select it and click "Edit"
8. Click "New" then enter "C:\llvm-mingw\bin"
9. Make sure "C:\Go\bin" is in the Path list. If not, add it by pressing "New"

** If you have VSCode or terminals open, close & restart them to get the new environment variables before proceeding **

10. In a PowerShell terminal run "go install fyne.io/fyne/v2/cmd/fyne@latest"
11. Run "fyne version" in the terminal. If you get output such as "fyne cli version: v2.5.3", then you have succeeded

12. Install VSCode from https://code.visualstudio.com/ if you haven't already
13. Install Go extension from go.dev in VSCode
14. In VSCode press Ctrl+Shift+P and type "go install tools". Select the first alternative. Check all boxes and click "OK"

15. Code!