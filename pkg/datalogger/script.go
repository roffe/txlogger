package datalogger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/common"
	"github.com/roffe/txlogger/pkg/ebus"
	lua "github.com/yuin/gopher-lua"
)

// scriptWriter wraps a LogWriter and runs the user's Lua scripts for the
// active ECU against every frame before it is written, so script outputs land
// as columns in the same row they were computed from and on the ebus for live
// gauges and plotters.
//
// Scripts live in ~/txlogger/scripts/<ECU>/*.lua. Each script declares input
// and output channel names and an update function. Locals persist between
// frames, so state (previous values, filters, counters) is per script file:
//
//	inputs  = { "ActualIn.n_Engine" }
//	outputs = { "RPM.RoC" } -- rpm/s
//
//	local prev_rpm, prev_t
//	function update(v, t) -- v[name] = current value, t = unix milliseconds
//	  local rpm = v["ActualIn.n_Engine"]
//	  if prev_t and t > prev_t then
//	    out["RPM.RoC"] = (rpm - prev_rpm) * 1000 / (t - prev_t)
//	  end
//	  prev_rpm, prev_t = rpm, t
//	end
//
// Inputs must be channels present in the log (polled symbols or sysvars).
// A script that fails to load, references a missing input or raises a runtime
// error is disabled for the session with a message; logging itself never
// stops because of a script.
type scriptWriter struct {
	lw        LogWriter
	sysvars   *ThreadSafeMap
	onMessage func(string)
	scripts   []*luaScript
	resolved  bool
}

type luaScript struct {
	name     string
	L        *lua.LState
	update   lua.LValue
	vtab     *lua.LTable // input table passed to update, reused every frame
	out      *lua.LTable // global "out" the script writes results into
	inputs   []string
	outputs  []string
	inputCh  []*Channel // resolved against the channel list on first Write
	disabled bool
}

// newScriptWriter loads every script for cfg.ECU and returns a wrapping
// writer, or nil when there are no scripts to run.
func newScriptWriter(cfg Config, lw LogWriter, sysvars *ThreadSafeMap) *scriptWriter {
	dir, err := common.GetScriptsPath()
	if err != nil {
		return nil
	}
	files, err := common.ListFilesInPathByExtension(filepath.Join(dir, cfg.ECU), ".lua")
	if err != nil || len(files) == 0 {
		if err != nil && !os.IsNotExist(err) {
			cfg.OnMessage(fmt.Sprintf("scripts: %v", err))
		}
		return nil
	}
	sw := &scriptWriter{lw: lw, sysvars: sysvars, onMessage: cfg.OnMessage}
	for _, file := range files {
		s, err := loadScript(filepath.Join(dir, cfg.ECU, file), cfg.Symbols)
		if err != nil {
			cfg.OnMessage(fmt.Sprintf("script %s: %v", file, err))
			continue
		}
		cfg.OnMessage(fmt.Sprintf("script %s loaded: %s", file, strings.Join(s.outputs, ", ")))
		sw.scripts = append(sw.scripts, s)
	}
	if len(sw.scripts) == 0 {
		return nil
	}
	return sw
}

func loadScript(path string, symbols []*symbol.Symbol) (*luaScript, error) {
	L := lua.NewState()
	if err := L.DoFile(path); err != nil {
		L.Close()
		return nil, err
	}
	s := &luaScript{name: filepath.Base(path), L: L}
	fail := func(err error) (*luaScript, error) {
		L.Close()
		return nil, err
	}
	var err error
	if s.inputs, err = stringTable(L, "inputs", true); err != nil {
		return fail(err)
	}
	if s.outputs, err = stringTable(L, "outputs", false); err != nil {
		return fail(err)
	}
	// An output named like a polled symbol would produce two columns with the
	// same name; refuse it up front instead of writing a confusing log.
	for _, o := range s.outputs {
		for _, sym := range symbols {
			if o == sym.Name {
				return fail(fmt.Errorf("output %q collides with a logged symbol", o))
			}
		}
	}
	fn := L.GetGlobal("update")
	if _, ok := fn.(*lua.LFunction); !ok {
		return fail(fmt.Errorf("missing function update(v, t)"))
	}
	s.update = fn
	s.out = L.NewTable()
	L.SetGlobal("out", s.out)
	s.vtab = L.NewTable()
	return s, nil
}

// stringTable reads global name as a list of strings. A missing table is an
// empty list when optional (a script may have no inputs), an error otherwise.
func stringTable(L *lua.LState, name string, optional bool) ([]string, error) {
	tbl, ok := L.GetGlobal(name).(*lua.LTable)
	if !ok {
		if optional {
			return nil, nil
		}
		return nil, fmt.Errorf("missing %q table", name)
	}
	var out []string
	tbl.ForEach(func(_, v lua.LValue) {
		out = append(out, v.String())
	})
	if !optional && len(out) == 0 {
		return nil, fmt.Errorf("%q table is empty", name)
	}
	return out, nil
}

// outputNames returns every output declared by the loaded scripts, in script
// order. buildChannels appends these as sysvar columns.
func (sw *scriptWriter) outputNames() []string {
	var names []string
	for _, s := range sw.scripts {
		names = append(names, s.outputs...)
	}
	return names
}

// resolve binds each script's declared inputs to channels. Runs on the first
// Write, when the channel list is final.
func (sw *scriptWriter) resolve(channels []Channel) {
	byName := make(map[string]*Channel, len(channels))
	for i := range channels {
		byName[channels[i].Name] = &channels[i]
	}
	for _, s := range sw.scripts {
		for _, name := range s.inputs {
			ch, ok := byName[name]
			if !ok {
				s.disabled = true
				sw.onMessage(fmt.Sprintf("script %s disabled: input %q is not logged", s.name, name))
				break
			}
			s.inputCh = append(s.inputCh, ch)
		}
	}
	sw.resolved = true
}

func (sw *scriptWriter) Write(ts time.Time, channels []Channel) error {
	if !sw.resolved {
		sw.resolve(channels)
	}
	t := lua.LNumber(ts.UnixMilli())
	for _, s := range sw.scripts {
		if s.disabled {
			continue
		}
		for i, ch := range s.inputCh {
			s.vtab.RawSetString(s.inputs[i], lua.LNumber(ch.Value()))
		}
		if err := s.L.CallByParam(lua.P{Fn: s.update, NRet: 0, Protect: true}, s.vtab, t); err != nil {
			s.disabled = true
			sw.onMessage(fmt.Sprintf("script %s disabled: %v", s.name, err))
			continue
		}
		for _, name := range s.outputs {
			if n, ok := s.out.RawGetString(name).(lua.LNumber); ok {
				v := float64(n)
				sw.sysvars.Set(name, v)
				ebus.Publish(name, v)
			}
		}
	}
	return sw.lw.Write(ts, channels)
}

func (sw *scriptWriter) Close() error {
	for _, s := range sw.scripts {
		s.L.Close()
	}
	return sw.lw.Close()
}
