# 2.0.9

- Set min/max values for MAF.m_AirFromp_AirInlet to match other airmass values in the plotter
- fix bug when using page up and down would not advance more than one step in logplayer
- added setting to use AD Scanner as ECU lambda source

# 2.0.8 

- Added ESP calibration settings for T7. Found under "Calibration" in the menu
- Updated to latest goCAN

# 2.0.7 Zero conf drivers für alles

The new FTDI d2xx driver has been implemented to leverage zero conf for several adapters.  
The OBDLink SX & EX and CANUSB will be autodetected on startup and all you need to do is select the driver starting with "d2xx" in CAN settings. No more selecting ports or setting latency in device manager.

  - Added FTDI d2xx driver
  - fixed mouse panning in mesh viewer so it doesn't behave strange after you rotate the mesh

# 2.0.6 3d updates baby

  - FINALLY fixed the cameras on the 3d mesh view. Now it behaves like any normal 3d software and is very intuitive to use. Mouse1, 2 & middle are the modifiers to use when dragging
  - 64 bit j2534 support added in gocan, Devices are prefixed "x64 J2534" and should be used if you see both 32-bit and 64-bit drivers for your adapter in the list
  - fixed a bug in the j2534 driver where 4 bytes would be appended to the can packages

# 2.0.5 CAN library rework

- Rewrote large parts of the CAN library to pass along a pointer to a message instead of a interface with methods to lower cpu usage

# 2.0.4 CANUSB optimization

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