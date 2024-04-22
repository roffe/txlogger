# 1.0.15

- Presets are NOT saved autmaticly on exit. If you have made changes to the presets you need to save them manually from the settings menu as a new preset or overwrite an existing one or your changes will get lost
- Added support for Trionic 5 (yay!)  
  Support for Trionic 5 is still in beta, please report any issues you find
- Added support for using OBDLink cables with Trionic 5  
  Tested and working devices are OBDLink SX & EX and STN2120, STN1170 "should" work but is untested

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