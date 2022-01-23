package main

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type actionType int

const (
	dv float64 = -1

	actionTypeStart = 0

	actionTypeEnd = 1

	actionTypeIng = 2

	actionPass = "pass"

	actionSkip = "skip"

	actionFail = "fail"

	actionPause = "pause"

	actionCont = "cont"

	actionBench = "bench"

	actionOutput = "output"

	actionRun = "run"

	// printed by test on successful run.
	bigPass = "PASS\n"

	// printed by test after a normal test failure.
	bigFail = "FAIL\n"

	// printed by 'go test' along with an error if the test binary terminates
	// with an error.
	bigFailErrorPrefix = "FAIL\t"

	updatesRun   = "=== RUN   "
	updatesPause = "=== PAUSE "
	updatesCont  = "=== CONT  "

	reportsPass  = "--- PASS: "
	reportsFail  = "--- FAIL: "
	reportsSkip  = "--- SKIP: "
	reportsBench = "--- BENCH: "

	fourSpace = "    "

	skipLinePrefix = "?   \t"
	skipLineSuffix = "\t[no test files]\n"
)

// TestEvent {"Time":"2022-01-23T16:58:49.186901+08:00","Action":"output","Package":"modify","Package":"modify.init.0()\n"}
type TestEvent struct {
	Action     string     `json:"Action" xml:"action,attr,omitempty"`
	Package    string     `json:"Package,omitempty" xml:"package,attr,omitempty"`
	Test       string     `json:"Test,omitempty" xml:"name,attr,omitempty,comment=测试名"`
	Output     string     `json:"Output,omitempty" xml:"output"`
	Elapsed    float64    `json:"Elapsed,omitempty" xml:"-"`
	Time       *time.Time `json:"Time,omitempty" xml:"-"`
	index      int
	actionType actionType
}

type Count struct {
	Total int `xml:"total,attr"`
	Pass  int `xml:"pass,attr"`
	Skip  int `xml:"skip,attr"`
	Bench int `xml:"bench,attr"`
	Fail  int `xml:"fail,attr"`
}

type TestInfo struct {
	XMLName xml.Name   `xml:"all"`
	TpList  []*TestPkg `xml:"pkg"`
	Time    time.Time  `xml:"xml-create-time,attr"`
	*Count
}

func (ti *TestInfo) setCount() {
	for _, testPkg := range ti.TpList {
		ti.Total += testPkg.Total
		ti.Pass += testPkg.Pass
		ti.Bench += testPkg.Bench
		ti.Skip += testPkg.Skip
		ti.Fail += testPkg.Fail
	}
}

type TestUt struct {
	TestEvent
	StarTime string `json:"-" xml:"star-time,attr"`
	EndTime  string `json:"-" xml:"end-time,attr"`
	Dur      string `json:"-" xml:"dur,attr"`
}

func (u *TestUt) initTime() {
	dur := time.Duration(u.Elapsed * float64(time.Second))
	u.EndTime = u.Time.Format("15:04:05.000")
	u.StarTime = u.Time.Add(dur).Format("15:04:05.000")
	u.Dur = dur.String()
}

type TestPkg struct {
	*TestUt
	teMap  map[string][]*TestEvent
	TEList []*TestUt `xml:"ut"`
	*Count
}

func (tp *TestPkg) init() error {
	for testName, events := range tp.teMap {
		e := &TestUt{TestEvent: TestEvent{Test: testName}}
		tp.TEList = append(tp.TEList, e)
		var action string
		for _, event := range events {
			e.Output += event.Output
			if event.actionType == actionTypeStart {
				e.index = event.index
				e.Package = event.Package
			}

			if event.actionType == actionTypeEnd {
				e.Elapsed = event.Elapsed
				e.Action = event.Action
				e.Time = event.Time
				e.actionType = actionTypeEnd
				action = event.Action
			}
		}
		e.initTime()
		err := tp.setCount(action)
		if err != nil {
			return err
		}
	}
	tp.Total = len(tp.TEList)
	return nil
}
func (tp *TestPkg) setCount(action string) error {
	switch action {
	case actionSkip:
		tp.Skip++
	case actionPass:
		tp.Pass++
	case actionFail:
		tp.Fail++
	default:
		if len(action) < 1 {
			return errors.New("action获取错误")
		}
	}
	return nil
}

func main() {
	_, err := os.Stdin.Stat()
	if err != nil {
		log.Fatalln(err)
	}
	decoder := json.NewDecoder(os.Stdin)
	var tlList []*TestEvent
	index := 0
	for decoder.More() {
		var tE = TestEvent{Elapsed: dv, index: index}
		index++
		tlList = append(tlList, &tE)
		err := decoder.Decode(&tE)
		if err != nil {
			panic(err)
		}
	}
	pkgMp := map[string][]*TestEvent{}
	for _, event := range tlList {
		err := event.setActionType()
		if err != nil {
			panic(err)
			return
		}
		pkgMp[event.Package] = append(pkgMp[event.Package], event)
	}
	t := &TestInfo{Count: &Count{}, Time: time.Now()}
	for pkg, events := range pkgMp {
		tp := &TestPkg{TestUt: &TestUt{}, teMap: map[string][]*TestEvent{}, Count: &Count{}}
		tp.Package = pkg
		t.TpList = append(t.TpList, tp)
		for _, event := range events {
			if len(event.Test) < 1 {
				tp.Output += event.Output
				if event.actionType == actionTypeEnd {
					tp.Action = event.Action
					tp.Time = event.Time
					tp.index = event.index
				}
				if event.hasElapsed() {
					tp.Elapsed = event.Elapsed
					tp.initTime()
				}
			}
			if len(event.Test) > 0 {
				tp.teMap[event.Test] = append(tp.teMap[event.Test], event)
			}
		}
		err := tp.init()
		if err != nil {
			panic(err)
			return
		}
	}
	t.setCount()
	t.writeToXml()
}

func (ti *TestInfo) writeToXml() {
	sort.Slice(ti.TpList, func(i, j int) bool {
		return ti.TpList[i].index < ti.TpList[j].index
	})
	bts, err := xml.MarshalIndent(ti, "", "\t")
	if err != nil {
		panic(err)
		return
	}
	path := filepath.Join(os.TempDir(), "cov", "cov.xml")
	err = os.MkdirAll(filepath.Dir(path), os.ModePerm)
	if err != nil {
		panic(err)
		return
	}
	err = os.WriteFile(path, append([]byte(xml.Header+"\n"), bts...), os.ModePerm)
	if err != nil {
		panic(err)
		return
	}
	log.Println(path)
}

func (e *TestEvent) setActionType() error {
	switch strings.TrimSpace(e.Action) {
	case actionRun:
		e.actionType = actionTypeStart
	case actionFail, actionPass, actionSkip:
		e.actionType = actionTypeEnd
	case actionOutput, actionPause, actionCont, actionBench:
		e.actionType = actionTypeIng
	default:
		return errors.New("未处理的actionType: " + e.Action)
	}
	return nil
}

func (e *TestEvent) hasElapsed() bool {
	return e.Elapsed != dv
}
