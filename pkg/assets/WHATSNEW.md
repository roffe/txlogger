# 2.1.10
- Added "Modern" gauge style selectable under graphics settings. It now covers the bar gauges too: VBar, HBar and CBar get a rounded dark track, a solid green→yellow→red fill and labels moved off the bar so they stay readable
- Added "Compare symbols with other binary" under the Tools menu. Pick a second binary of the same ECU type and get a list of every symbol whose data differs from the currently loaded one, with symbol number, size and address (size mismatches are flagged). Click a symbol to preview it, double-click to keep it open in its own tab. Each symbol shows three tabs: a Diff tab with the per-cell difference (current - other), Current and the other binary. Logging must be stopped while comparing
- The refresh button in CAN settings now also rescans for adapters, so devices plugged in (or J2534 drivers installed) after txlogger started are picked up without a restart
- Fixed the mapviewer grid so all cells and the gaps between them render with consistent sizes
- Reworked inner window resizing: grab any edge, and the ends of each edge act as diagonal corner resize like native windows
- Cut a new logfile from a selection in the logplayer: scrub to a spot and press "In" (or the `i` key) to mark the start, scrub again and press "Out" (or `o`) to mark the end, then press the save button to write just that range to a new log next to your other logs. The clip keeps the same format as the source log (csv/bpl/t5l/t7l/t8l). Leaving the In or Out point unset selects from the start or to the end of the log
- Live tracking marker in the 3d mesh viewer showing where the ECU is reading from, mirroring the crosshair in the map above
- Fixed the 3d mesh showing one cell less than the table in each direction; values are now cell-centered so an 18x16 map renders 18x16 cells
- Performance optimization for the meshgrid
- Force layouts to be loaded and saved in users home directory under the txlogger folder
- Update ecusymbol to be able to read T5 versions
- Added new config widget for AD scanner WBL settings inspired by T7's DisplAdap.LamScannerTab
- Refactored the bus implementation to use less CPU and have less allocations
- Added support for BPL files ( binary packed logfile )
- Removed support for creating legacy TXL log files. (you can still load them but might cause crashes)
- Removed ebusmonitor, it has served it's purpose
- Improved drag handler in logplayer, when zoomed in we drag fewer frames increasing as we zoom out
- We now have 3 render modes for viewing 3d maps, Solid Wireframe, Solid & Wireframe. Press the little square icon in the mesh viewer to switch between them
- WBL reconnect COM port while logging. If the COM port dies for a reason during logging it will try to re-connect
- Performance improvements in many widget to allow slower computers to run txlogger better
- Improved camera handling in the 3d mesh viewer - now behaves like the t5/7/8 suites
- Added 2D graph for viewing flat maps
- Rewrote logplayer plotter to use about 50% less CPU on zoomed out views
- Big refactor of the log writing logic to be simpler to maintain and be more performant
- Improved cell selection in mapviewer
- Improved copy paste in mapviewer, added paste here function
- Added a Matrix builder from logfiles. It learns a 2D map from one or more logs: pick which series drives the X axis, the Y axis and supplies the Z value, and every sample that lands on a cell is averaged into it. The result is shown live in a mapviewer (colored grid + 3D mesh) and the cells can be edited by hand
  - Load and merge multiple log files at once (t5l, t7l, t8l, csv, bpl); series are row-aligned across files
  - Pick X/Y/Z from a dropdown of the loaded series or type a name by hand
  - Adjustable column/row counts and fully editable axis breakpoints, with an "Auto" button that spreads a series' min..max evenly across an axis
  - Per-axis Z-hit tolerance sliders: reject samples that sit too far from a breakpoint so only values close to a cell count toward it
  - Visual filter / query builder: add rules like "if <series> <op> <value>" and a sample only counts as a hit when it satisfies every rule. Operators: >, >=, <, <=, ==, != and ~ (approximately equal)
  - Filter query language: instead of the visual rules you can type a full query with and/or, () grouping and the same operators, e.g. "if (ActualIn.n_Engine > 3000 and Out.X_AccPedal > 50) or boost ~ 1.2". Series can be compared to numbers, to each other or to arithmetic of them; a non-empty query overrides the rules
  - Save and load configurations as presets (series, dimensions, axis breakpoints, tolerances and filter rules)
- Added a bunch of TransCal maps under Fueling on T7
- Replaced gocangateway with a slimmer j2534proxy
- Rewrote everything to use GoCAN v2

# 2.1.9
- Updated default T7 preset to include MAF.m_AirFromp_AirInlet

# 2.1.8
Development is now done on Linux as i've decided to permanently ditch Windows once and for all. There will still be Windows builds

- Linux Drewtech Mongoose GM II driver, it's a highly experimental driver from reverse engineering their binary protocol enabling logging of T7 & T8 on linux

Known issues:
- d2xx OBDLink ftdi driver does not work for OBDLink devices on Linux so do not use 

# 2.1.7
- Added "Edit Parameters" dialog for Trionic 8 under Diagnostics menu. Allowing you to change VIN, E85%, Top Speed, Oil Quality, Diagnostic Type, Tank Type, Convertible, SAI, High Output, BioPower and ClutchStart
- Fixed a possible crash in the combined logplayer

# 2.1.6
- Added all maps from "Manual tuning" in T5suite
- Added all maps from "Tuning" in T7suite
- Fixed small graphical discrepancy where the shadow around windows would be 2 pixels off
- Updated txbridge firmware to 1.1.1 which includes write support for Trionic 5
- Fixed a bug where the working directory would be changed when opening file via dialogs on Windows
- txbridge firmware updated so it uses the slow write to ram method for Trionic 5 that should work with engine running

# 2.1.5
- Added numbers on the gauges in the dashboard to look more like real gauges
- Fixed axis information for tryck_mat_a! in ecusymbol library where it would open tryck_mat! instead
- Added a small device check and error out if trying to use J2534 or ELM327 adapters with Trionic 5
- Fixed a bug in the update checker where the update check would not be performed properly
- Fixed a bug where you could rotate the 3d mesh of a map view when resizing a window overlaying it
- Added dragable borders to all windows to make resizing easier
- Added DTC reader for Trionic 5, 7 & 8. Found under the "Diagnostics" menu

# 2.1.4
- Symbollist now remembers the last selected preset per ECU.
- Fixed a bug where logs loaded from menu would be stuck in the top left corner
- Tweaked positioning when dragging and dropping log files in the main window
- Created driver for PCAN adapters on Windows using the PEAK Basic API DLL. This enables T5 support with PCAN adapters.
- Rewrote large parts of goCAN to have better error handling
- Fixed a bug in the Kvaser Canlib implementation making it possible to move it out of the cangateway back into the main application
- Fixed crash where app would not start it T7 ECU was selected and WBL was set to ECU Lambda source

# 2.1.3
- Added a check for updates dialogue that will show every second week.
- Fixed a bug where the user defined log path would not be adhered to after changing it in settings
- Bunch of more memory optimizations in dataloggers to lower GC pressure
- Fixed a bug where the color blind mode was not applied on opening new maps
- Rewrote settings dialogue to be easier to extend and maintain
- Added support for saving T5 files

# 2.1.2
- Rewrote cangateway to use named pipes on windows instead of unix sockets. This should ensure that cangateway is working even on early Windows 10 versions
- Fixed a race condition in goCAN that could cause missed canbus frames.

# 2.1.1
- Fixed a bug where the background color of single cell maps (bool values) would be black until selected
- Added support for writing to SRAM on Trionic 5 ( you can now livetune T5 with txlogger )
- Now possible to change Pgm_mod! in SRAM on T5
- Fixed timing bug where writing to ram on T8 would fail some times

# 2.1.0
- Added support for txbridge discovery via mDNS.
- txbridge firmware now supports AP, STA or AP+STA modes. Configurable via the configurator widget under settings.
- Tweaked the symbollist to have bigger preview bars
- Bug fixes and optimizations
- Added color themes for different color blindness. Changeable under settings
- Improved camera controls in the 3D Mesh viewer

# 2.0.9
- Set min/max values for MAF.m_AirFromp_AirInlet to match other airmass values in the plotter
- fix bug when using page up and down would not advance more than one step in logplayer
- added setting to use AD Scanner as ECU lambda source

# 2.0.8 
- Added ESP calibration settings for T7. Found under "Calibration" in the menu
- Updated to latest goCAN

# 2.0.7
The new FTDI d2xx driver has been implemented to leverage zero conf for several adapters.  
The OBDLink SX & EX and CANUSB will be autodetected on startup and all you need to do is select the driver starting with "d2xx" in CAN settings. No more selecting ports or setting latency in device manager.

- Added FTDI d2xx driver
- fixed mouse panning in mesh viewer so it doesn't behave strange after you rotate the mesh

# 2.0.6
- FINALLY fixed the camera on the 3d mesh view. Now it behaves like any normal 3d software and is very intuitive to use. Mouse1, 2 & middle are the modifiers to use when dragging
- 64 bit j2534 support added in gocan, Devices are prefixed "x64 J2534" and should be used if you see both 32-bit and 64-bit drivers for your adapter in the list
- Fixed a bug in the j2534 driver where 4 bytes would be appended to the can packages

# 2.0.5
- Rewrote large parts of the CAN library to pass along a pointer to a message instead of a interface with methods to lower cpu usage

# 2.0.4
- Added support for Lawicel CANUSB DLL. No more fiddling with VCP and latencies. required 64-bit DLL is included with txlogger.
- Moved back all CAN communications except for J2534 DLL's to the main program to not incur performance pentaly of using cangateway when not necessary
- Updated libusb to 64-bit for use with CombiAdapter
- Updated Kvaser drivers to use 64-bit
- Added ECU dump & info on all 3 Trionic versions (no txbridge support yet) 

# 2.0.3
- Optimized most adapter drivers in goCAN

# 2.0.2
- Fixed a bugg where the knock icon would not hide after a few seconds on the dashboard
- Huge rewrite of the goCAN canbus drivers to have better error handling and a clearer path on how to propagate messages to the UI
- Started adding support for dumping and flashing ECU's, dumping and info should work on all 3 platforms. (no txbridge support yet)

# 2.0.1
- Improved kvaser CANlib drivers in goCAN
- Fixed so Lambda.External's value is properly displayed in plotter legend
- txbride firmware updater now supports both wifi and bluetooth.  
  To update the firmware from Bluetooth to wifi select "txbridge bluetooth" as device in CAN settings and select the corresponding bluetooth port then update the firmware from the file menu.  
  After the firmware has been updated your txbridge will create a wifi hotspot with the same name the Bluetooth device had.  
  Change the CAN device to "txbridge wifi" and connect to the wifi network with password **123456789**. after that you can continue logging as before.

# 2.0.0
This is a huge milestone release. 

The user interface has been competely revamped to allow inline windows, custom gauges and plotters to be created, moved around and layouts saved & restored.

The logplayer has moved into the main UI and starts with a plotter & playback controls. You are then free to open a Dashboard if you want one or view the values in the symbol list.
Or why not create your own gauges and make it just like you want

- Competely new UI - most windows & maps now opens inside the main window and is resizeable and arrangeable
- Reworked legend to have a more "fixed size" and value moved to the left
- Fixed scaling of IOFF x-axis when live viewing BstKnkCal.MaxAirmass on T8
- Added t8 pedal map to Torque menu
- Added the possibility to add custom gauges and meters and build your own dashboard on the main screen
- Added functionality to save "layouts" which can be a set of open maps and different configured gauges. These can then be easily swapped between when for example playing logs or live-tuning
- Added "in-line" logplayer reachable from the play button in the bottom right corner.
- Fixed bug where mReq and mAir could have different starting points in log plotter
- Added EBUS monitor to see what messages are flying around in the internal bus
- Now possible to select multiple different cells by holding CTRL and clicking
- Logplayer rewritten to use a lot less CPU and be more responsive
- This is now a single instance application. If you try to open log files from file associations when txlogger is running it will open them in the running instance instead
- Drag & Drop support improved. The logplayer / plotter for the logfile will now open under the mousepointer where it was dropped
- New settings dialogue
- New default filename for logs. The filename will now be prefixed with the name of the binary you have loaded when logging.
- Symbol preset management has now been moved into the symbollist dialogue
- Moved txlogger firmware update shortcut to "File"
- Added "What's new" to "File" menu to access this document
- Added support to drag the plotter instead of having to use the slider to seek in the logfile
- Improved T5 support
- goCAN now supports Kvaser Canlib for all Kvaser products
- The CANbus communication has been broken out to a separate binary that is compiled as 32-bit due to the requirements for j2534 dll's.

# 1.0.19
- Added E85.X_EthAct_Tech2 to Trionic 7 calibration shortcuts
- Added T5 support for TXbridge
- Improved TXbridge T8 support
- Added OTA firmware update for txbridge
- Trionic 7 & 8: Added support for offloading read & write by memory address to TXbridge
- When hovering over symbols in the legend for the plotter, the symbol will be highlighted in the plot
- Hovering over labels in the log player plotter will make them bold and make the series' drawn line thicker
- Hovered labels will also be shown in large text to the left on the plotter
- A ton of performance optimizations
- Reworked most widgets in the dashboard so they can scale much smaller
- Made log player plotter resizable even on low-resolution monitors

# 1.0.18
- Add code to convert T5 AD_EGR value to lambda 0.5 - 1.5
- Add settings to configure WBL when reading AD values from T7
- Fixed bugg where IDC did not change color on threshold values
- Tweaked border around wbl, nbl, turbo pwm and tps gauges
- Tweaks to the dashboard widgets to use less cpu
- Adjusted minimum line width in the gauges in the dashboard
- Added support for serial logging of Innovate wideband controllers (MTX-L & LC-2) & AEM Uego with usb <-> serial adapter
- Added support for CAN logging of AEM Uego Wideband controllers
- Added AMUL to Trionic 7 preset and dashboard
- Initial support for txbridge
- Switched from TDM-GCC to MingW64 for building
- Greatly reworked the 3d mesh viewer for maps (camera controlls still isn't great, but better)
- Solved problem with no console output when launched from terminal in Windows
  this will greatly help debugging and troubleshooting. If you have problems with crashes
  start txlogger with the debug.bat file and create a issue on Github or forum post on TrionicTuning.

# 1.0.17
- If WBL is set to None the WBL will not be shown in logplayer
- Changed color of crosshair in mapviewer to make it easier to see
- Fixed a bug where pedal position was not properly translated to pedalmap in Trionic 7
- changed scaling of AirCompCal.PressMap to bar instead of kPa

# 1.0.16
- Fixed bug where some t5 files would not load
- Added support for drag and drop loading of binaries and logs
- Fixed bugg where ioff would not be visualized properly in map viewer

# 1.0.15
- Presets are NOT saved autmaticly on exit. If you have made changes to the presets you need to save them manually from the settings menu as a new preset or overwrite an existing one or your changes will get lost
- Added support for Trionic 5 (yay!)  
  Support for Trionic 5 is still in beta, please report any issues you find
- Added support for using OBDLink cables with Trionic 5  
  Tested and working devices are OBDLink SX & EX and STN2120, STN1170 "should" work but is untested
- Added support for registering Myrtilos binaries over CANBUS

# 1.0.14
- Moved CANBUS adapter settings from main screen into settings
- OHM ( One Hand Mapping ) has been added. if you enable "Cursor follows crosshair in mapviewer" under settings the cursor for editing will now follow the crosshair in the mapviewer. This makes it possible to edit maps with one hand while driving. a & z for minor increment and s & x for major increment.
- Fixed colors for certain symbols in plotter
- Code optimization
- Dual dial secondary needle is now red to make it easier to see
- fixed bug where the logplayer button would not open a file browser in the directory set under settings
- fixed so AirCtrlCal.Regmap is using m_Req instead of m_Air to show crosshair in mapviewer

# 1.0.13
The default presets has been updated. Be sure to load it once from the settings menu to make sure ActualIn.n_Engine, Out.X_AccPedal & In.v_Vehicle is logged properly on Trionic 7

In earlier versions there existed different presets depending on your CAN adapter. This has been fixed and the presets are now the same for all adapters. The default presets has been updated to reflect this change

- Added WHATSNEW.md that will be displayed once the first time a new version is started.
- A ton of code optimizations to make the Dashboard & logplayer use less cpu
- Added ignition duty cycle (Idc) to Dashboard, loggable via Myrtilos.InjectorDutyCycle once EU0D v25 is released, display value is 0 - 100%
- Fixed a bugg in the symbol list where "ghost" duplicates of symbols would be added when the same symbol was added to the list multiple times
- Changed symbol name in symbol list to be a label instead of a textbox, also added a copy symbol name button on each row
- Added additional symbols to Trionic 7 main menu
- It's now possible to create your own presets selectable from the preset dropdown
- Added a Log plotter in the log player so you can see line graphs of the recorded values

# 1.0.12
Mostly under the hood fixes

- some huge rewrites in the internal data processing which resulted in about halved CPU usage,
If one turns off real-time preview values in settings it uses less than 0.5% CPU on my dev machine when logging.
- bugg fixes for STN adapters, they should be more stable now
- optimized CANUSB driver so it uses less CPU
- better memory management and reuse of graphics elements instead of recreating them each time shown
- The T7 presets has been merged into one, no more having to have different presets depending on what cable you have. txlogger will now solve this under the hood

# 1.0.6
- New settings dialogue
- Possible to copy paste map data between t7suite <-> txlogger
- You can type in values when editing maps
- Edit multiple cells at the same time
- Can load symbols and maps from binary
- Can load and save maps from ram on open T7 bins
- Setting to autoload maps from ECU ram ( requires loaded open bin for axis information )
- Right click menu for copy & paste and smooth operation on maps
- A lot of code has been written for reading and writing ECU ram on open T7 bins
- Ton of rewritten code for stability and performance
- Better responsiveness in map viewer.
- new 3D map viewer
- support for editing maps in t7 binaries
- can update and verify t7 binary checksums
- reworked settings & real-time symbol list
- ability to on the fly change symbols without having to restart logging
- read and write sram maps on T7 with open bins
- copy paste between t7/8 suite and txlogger

# 1.0.5
Ever wondered how the ECU interpolates values from the maps in the binary live? Now you don't have to. With our all-new map viewer function, get a real-time view of the process. It’s visual, it’s intuitive, and it’s designed to provide insights like never before.

Major Under-the-Hood Improvements!

Your favorite logger just got faster and more efficient. Dive into the details and you'll find a massive code refactor that paves the way for:
Significantly Reduced CPU Usage: Whether you're logging or using the dashboard & logplayer, expect a silky-smooth performance with reduced strain on your CPU.

Other Updates & Fixes:

We've also made some minor bug fixes and UI enhancements for a polished user experience.

We're always working to make txlogger better for our community. Thanks for being on this journey with us.

# 1.0.4
File Association Improvement: Now you can effortlessly associate .t7l and .t8l files with txlogger.exe. When opening these file types, txlogger will directly launch the logplayer, eliminating the need for manual steps like browsing and clicking play logs. To set up the file associations, run setup.exe or right-click the files and select "Open With," then browse for txlogger.exe.

Enhanced Date & Time Parser: Our log player now boasts an improved date and time parser, catering to multiple date standards. No more worries about compatibility issues—enjoy a seamless log playback experience!

Optimized Log Player Code: The log player code has been optimized to reduce CPU usage by pre-parsing logs before playback, ensuring an efficient and smooth log viewing experience.

Upgrade to txlogger version 1.0.4 today and make managing and playing logs a breeze. Download now and elevate your logging efficiency!

Happy Logging!

# 1.0.3
- Added a shiny new "logs folder" button
- Shorter log filenames for your convenience
- Upgraded our symbol libraries to read maps like a champ
- Leveled up KWP library to read maps from RAM (get ready for some cool stuff!)
- Updated our GUI framework to the latest and greatest version
- When loading log files, start in the "logs" folder next to txlogger.exe

# 1.0.2
- More performance optimization

# 1.0.0
- Renamed to txlogger
- Gotten T8 support
- Support for old T7 binaries with the 14 bytes address table and uncompressed symbol table
- A homepage has been created: https://txlogger.com
- More keyboard shortcuts added, see help in software
- A lot of performance optimization
- Graduated to 1.0.0 release with the T8 addition

# 0.0.7
- Fixed crash when trying to load symbol name table from ECU running BIN without symbol names
- Added symbols for EU0AF01O
- Knock warning on dashboard if logging "KnkDet.KnockCyl"
- Support for loading XML schemas on binaries with no name table
- Keyboard shortcuts added to log layer, dashboard and main window ( press help in main screen to see all shortcuts )
- Various UI polish (better scaling and responsiveness)