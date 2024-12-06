# 1.0.19

- Added E85.X_EthAct_Tech2 to Trionic 7 calibration shortcuts
- Added T5 support for TXbridge
- Improved TXbridge T8 support
- Added OTA firmware update for TXbridge
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