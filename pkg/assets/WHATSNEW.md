# 2.2.0
- New "Actions" menu for Trionic 7, between Tools and Diagnostics, collecting the T7-only tools: Firmware information, Import address table from another binary, T7 Seed/Key patcher and (preview) T7 Boost Auto-Tuner
- Added "Firmware information" (T7Suite's dialog): shows the PI-area identity of the loaded binary and the calibration switches with checkboxes to flip them — open SID info (engine parameters viewable in the SID display), torque limiters, OBDII, second lambda sonde, fast and extra fast throttle response, catalyst light-off, BioPower and ethanol sensor, plus the EU0AF01C-only SID start screen/adaption message and emission limiting patches. Options the binary doesn't carry are greyed out. Apply writes the changes with a one-time .bak of the original and recomputes the checksum. T7Suite's "No TCS" checkbox is the ethanol sensor switch and is labelled as such here
- Added "Import address table from another binary" under the Actions menu (T7)
- T7 symbol names for more software: EU06 (EU06F01C/Z44C/Z44O), EU04Z40O, EU03Z36C/O, ET07F02C, EU02Z30O/Z32O, ET06F01C and EI03F01C now load with names instead of failing with "unknown xml". EU06Z44O previously loaded with the wrong names
- CAN flasher: Trionic 8 (Legion) erase now says which partitions were tagged by the MD5 comparison and which ones are being erased, and reports elapsed time every 5 seconds while the loader is busy plus the total when it finishes, instead of a bare "Erasing device: 6" followed by silence
- Added "Hex editor" under the Tools menu: a raw hex view of the loaded binary with byte-level editing in both columns. Click a byte in the hex column and type hex digits to change it nibble by nibble, or click in the ascii column and type text straight in; the cursor byte is highlighted in both columns at once, Tab or clicking switches which column edits (the active one is framed), and the bytes-per-row width is selectable (8/16/24/32). Arrows/PgUp/PgDn/Home/End navigate, Ctrl+Home/End jump to the start/end of the file. Goto takes a hex address, find takes hex bytes or plain text and wraps around. The status bar shows the offset, value and which symbol the cursor is inside. Backspace (or the Undo button) reverts one byte at a time. Edits made inside a symbol's region update that symbol too, so nothing is lost when the file is saved — Save writes through the normal path, taking a one-time .bak of the original and recomputing checksums
- The Linux AppImage can now update itself: when "Check for updates" finds a newer release, an "Update AppImage now" button downloads it and replaces the running file in place. Restart txlogger afterwards to run the new version
- Added "Cam timing" under the Tools menu: a valve timing diagram with the duration, centerlines, lobe separation and overlap that follow from a cam's events, plus a Measured VE tab that works volumetric efficiency out of a T7/T8 log. Every stock SAAB 16v cam is a template along with the Swedish Dynamics and Catcams grinds, intake and exhaust pick independently, and every figure is editable for a cam that is in no list. Documented at https://txlogger.com/docs/cam-timing/
- Added "T7 MBT ignition analyser" under the Tools menu: works out where MBT timing lies across your ignition map and how far the map sits from it, so the cells that are pointlessly retarded stand out from the ones that are knock limited on purpose. It models the cylinder pressure of every cell of IgnNormCal.Map from the airmass and rpm axes, using the closed-form pressure model from Ingemar Andersson's 2002 Linköping thesis on ion-sense and cylinder pressure (the Mecel work behind Trionic's own ion sensing) — two polytropic asymptotes with a Vibe burn interpolating between them — and reports MBT timing, the gap to your map, the torque that gap costs, peak cylinder pressure and where in the cycle it happens. A second tab draws the modelled pressure trace for any rpm/load you type in. λ per cell follows the binary's own LambdaCal.MaxLoadNormTab so the enriched high-load region is modelled enriched. Engine geometry presets for B204, B205, B234 and B235 are included and every value is editable, because compression ratio in particular varies by variant. MBT itself is defined the way the literature does, as the crank angle a chosen fraction of the mixture should have finished burning at, defaulting to 45% at 9° ATDC. That particular choice is deliberate: Lars Eriksson's 1999 Linköping dissertation on spark advance measures how far each candidate descriptor drifts when the burn angles are wrong, and with the flame development and rapid burn angles moving together — which is how this model scales them — the 45% position drifts 1° where the 50% position drifts 3° and the peak pressure position drifts 15°, so the tool is built on the one descriptor that barely cares about the input it cannot measure. As a check the Cycle tab reports where 45%, 50% and peak pressure all land at the modelled MBT, against the values the literature expects; on a stock Aero map the 50% point lands at 10.3° with only the 45% point constrained, which is the sort of agreement that says the burn shape is right. Everything the model cannot know is a knob rather than a hidden constant — polytropic exponents, residual fraction and temperature, charge temperature, fuel heating value and stoichiometric ratio, the burn fraction and angle that define MBT, and the burn-angle model itself. Burn angle is the one that decides the answer and it is not measurable from a T7 log, so there is a "Fit to map" button that scales it against the part-load region of the loaded binary, where the calibration should already be at MBT and nothing is knock limited; the high-load deviation then means something. Treat the result as relative, not absolute: the shape across the map is trustworthy, the exact number in any one cell is only as good as the burn angles you fitted
- Fixed an AEM UEGO wired to the CAN bus never reading anything. The adapter's acceptance filter is widened for the wideband's CAN id 0x180 only when the WBL port is set to "CAN", but the check looked at the wideband *source* instead of the port, so the id was always filtered out and lambda sat at zero
- An AEM UEGO on a serial cable no longer feeds nonsense lambda into the log when the cable drops a bit. Cheap USB serial adapters (Prolific clones especially) corrupt the odd byte, so "10.3" arrives as "\x100.3" or " 0.3" — dropping the bad byte would have logged 0.03 lambda instead of 1.03, so the whole reading is now discarded and the next one used. The log no longer fills with a "invalid syntax" line per corrupt reading either; it says how many were discarded, every 25th one
- Removed "CombiAdapter" from the wideband source list. It was never implemented, and picking it aborted the whole logging session with "unknown WBL type: CombiAdapter" before the ECU was even contacted. If you had it selected, the source is reset to None
- Fixed Trionic 7 flashing failing at the very last step with "exit download mode failed: Busy, repeat request". The whole binary was written correctly, then the ECU was still programming the final block to flash when txlogger asked it to close the download — and "busy, repeat request" means exactly what it says: send it again. txlogger now does, for the download, block-close and EOL steps alike, instead of treating it as a failure. The same hiccup between blocks previously cost a full block retry. Routine replies are also now matched to the routine that was asked for (every routine answers with the same positive code, so a late reply could be mistaken for the next one's answer), and the erase reports how long it took
- CAN flasher: added an "EOL Flash" button for Trionic 7 that runs the real factory End-Of-Line programming sequence instead of a plain image reflash. On top of erase + program it writes the VIN (0x90), programming date (0x99) and tester serial (0x98), then runs end-of-procedure, which makes the ECU rewrite its Delco hardware numbers and security seed/key words itself, verify the flashed ROM checksum and set the EOL-success flag. The VIN is prefilled from the binary, the date defaults to today and the tester serial to "txlogger EOL" (the field records who ran the procedure, and the old value is shown as a hint). The binary's checksum is verified before anything is erased, so an inconsistent bin is rejected up front rather than aborting the ECU's end-of-procedure after the flash is already written. The tester serial is checked against the ECU's built-in kill-list — a handful of serial numbers make a T7 erase its own flash and halt, recoverable only over BDM — and those are refused
- The 3d mesh viewer no longer allocates memory on every frame while rotating or live-updating, and each update is ~15% faster
- Fixed the 3d mesh viewer's axis labels overprinting each other at certain rotation angles. The last tick of a scale is always labeled so the axis' full extent is readable, but when the labels were thinned it could land on top of its neighbor ("70006500"); the crowding neighbor now makes room. When the floor is viewed nearly edge-on, the two floor scales also printed into the same screen band; the shorter one now shifts outward so the scales stack instead. Shallow views also no longer run the vertical height scale straight through the middle of the surface, a top-down view no longer piles all its labels onto one point, and the scales no longer bounce between edges while dragging near symmetric angles
- Fixed the top values of VIOSMAFCal.Q_AirInletTab/Q_AirInletTab2 showing up negative. The T7 symbol table marks these MAF transfer tables as signed, but the ECU reads them as unsigned 16-bit, so anything above 32767 (the stock top value is 34000) wrapped negative in the mapviewer
- EU09F01C/EU09F01O/EU09F01T binaries now have a near complete symbol list. SAAB never shipped a compressed symbol name table in these late bins, so txlogger relied on a hand made XML that named 1810 of 4636 symbols — and all 1810 were calibration maps, which left every one of the 2328 RAM symbols unnamed and therefore unloggable. The names were reconstructed by aligning the EU09 symbol address table against 43 related T7 binaries that do carry an intact name table, cross checked against the EU03 build's own symbol definition file. 4632 of 4636 symbols are now named, including 2325 of 2328 RAM symbols, so the standard logging set (ActualIn.n_Engine, MAF.m_AirInlet, Lambda.LambdaInt, In.v_Vehicle, Out.X_AccPedal and the rest) is selectable at last. Reconstructed entries carry a confidence value in the XML; the handful of low confidence ones may still be misnamed
- The Trionic 7 datalogger now reads the SecurityAccess (0x27) XOR/SUB algorithm straight out of the loaded binary and tries it first, so logging an ECU flashed with a patched/custom algorithm just works — no manual entry needed. Falls back to the stock methods when the binary is a standard one or none is loaded
- Fixed Trionic 7 operations failing when run back to back. Reading info or dumping the ECU ends the diagnostic session when it finishes, but txlogger kept treating the session as open for another 8 seconds, so a second operation started in that window talked into a closed session and timed out. A failed connection attempt had the same effect, making an immediate retry do nothing. The session state now follows what was actually sent
- CAN flasher: you can now enter your own seed/key XOR and SUB (hex) when the selected ECU is Trionic 7. Use it for ECUs flashed with a patched SecurityAccess algorithm, which answer to none of the built-in methods so info/dump/flash fail at "Failed to obtain security access". Your pair is tried first and the built-in methods are still tried as a fallback, so leaving the boxes empty behaves exactly as before. Read the values out of the ECU's binary with Tools > T7 Seed/Key patcher; they are remembered between sessions
- Fixed corrupted maps in T7 binaries. These bins zero out the address of a handful of maps (BFuelCal.Map, IgnNormCal.Map, the TorqueCal maps and others) in the symbol address table, which made txlogger read them from the start of the file and render garbage. The addresses are now reconstructed from the surrounding table entries, same as T7Suite does
- The dashboard now follows the wideband source as you change it in settings, or when you switch ECU type, instead of showing a dead lambda gauge until you close and reopen it
- The symbol list value bars can now be turned on and off while it is open; the setting is renamed "Value bars in symbol list" and the dead "Live preview values in symbollist" checkbox next to it is gone
- CAN flasher: added a "Reset ECU" button. On Trionic 7 it warns first that the throttle body will go into limp mode if the ignition is on
- Customizable key shortcuts
- Added "T7 gear calculator" under the Tools menu: computes GearCal.Ratio / GearCal.Range values for the T7 manual gearbox calibration from gear ratios, final drive and tire diameter. Comes with FM55/FM57 templates, live recalculation as you type and a speed-per-gear graph with hover readout
- Added "NVDM editor" under Diagnostics for Trionic 8: edits the non-volatile data memory of the loaded binary — VIN, ECU hardware/software names, tester serial, programming date, Bosch part number, and the immobilizer block (securityCode, powerTrainSK, transponderSK, remoteControlSK, powerTrainIdentifier, securityLevel, ST_ImmoEnabled, free starts). NVDM is a ring of obfuscated 304 byte snapshots spread over two banks; the editor shows the newest one (picked by the bank generation counter, not by address, so it is the record the ECU actually reads) and writes the fields you changed to every snapshot in both banks so no stale copy can win. Fields you did not touch keep their exact bytes. A donor ECU that has been in more than one car still carries the older identities, and the editor says so before you overwrite them. There is no checksum over NVDM and the binary's own checksums start at 0x20000, so nothing else needs correcting. Reflash with "Unlock systems partition" enabled for the ECU to take the change
- Added "T7 Seed/Key patcher" under the Tools menu: reads the built-in SecurityAccess (0x27) key algorithm out of a T7 binary and lets you change the XOR and SUB values (pick a known method or enter custom hex). Useful for normalizing an ECU with an unknown algorithm so standard tools can unlock it. Checksums are recomputed on save

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
- Added color customizer to set custom colors on log plotter series
- Added edit functionality to dashboard so you can move gauges around freely

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