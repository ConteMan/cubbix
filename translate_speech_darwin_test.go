package main

import (
	"bytes"
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
)

type fakeAVSpeech struct {
	testing  *testing.T
	onMain   bool
	startErr error
	speaking bool
	created  int
	polled   int
	released []objc.ID
}

func (driver *fakeAVSpeech) requireMain() {
	driver.testing.Helper()
	if !driver.onMain {
		driver.testing.Fatal("native speech operation left the main thread seam")
	}
}

func (driver *fakeAVSpeech) Start(string) (objc.ID, error) {
	driver.requireMain()
	if driver.startErr != nil {
		return 0, driver.startErr
	}
	driver.created++
	return objc.ID(driver.created), nil
}

func (driver *fakeAVSpeech) IsSpeaking(objc.ID) bool {
	driver.requireMain()
	driver.polled++
	return driver.speaking
}

func (driver *fakeAVSpeech) StopAndRelease(synth objc.ID) {
	driver.requireMain()
	driver.released = append(driver.released, synth)
}

type fakeSpeechPoll struct {
	callback func()
	canceled bool
}

type avSpeechFixture struct {
	backend *avSpeechBackend
	driver  *fakeAVSpeech
	polls   []*fakeSpeechPoll
	clock   time.Time
}

func newAVSpeechFixture(t *testing.T) *avSpeechFixture {
	fixture := &avSpeechFixture{driver: &fakeAVSpeech{testing: t}, clock: time.Unix(1, 0)}
	fixture.backend = &avSpeechBackend{
		driver: fixture.driver,
		onMain: func(fn func()) {
			fixture.driver.onMain = true
			defer func() { fixture.driver.onMain = false }()
			fn()
		},
		now: func() time.Time { return fixture.clock },
		after: func(delay time.Duration, fn func()) func() {
			if delay != speechPollInterval {
				t.Fatal("unexpected polling interval")
			}
			poll := &fakeSpeechPoll{callback: fn}
			fixture.polls = append(fixture.polls, poll)
			return func() { poll.canceled = true }
		},
	}
	t.Cleanup(fixture.backend.Stop)
	return fixture
}

func (fixture *avSpeechFixture) tick(elapsed time.Duration) {
	fixture.clock = fixture.clock.Add(elapsed)
	fixture.polls[len(fixture.polls)-1].callback()
}

func TestAVSpeechPollingLifecycle(t *testing.T) {
	for _, mode := range []string{"natural", "missed short utterance", "maximum duration"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newAVSpeechFixture(t)
			backend, driver := fixture.backend, fixture.driver
			completions := 0
			if err := backend.Speak("hello", func() { completions++ }); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "natural":
				driver.speaking = true
				fixture.tick(speechPollInterval)
				driver.speaking = false
				fixture.tick(speechPollInterval)
			case "missed short utterance":
				fixture.tick(speechPollInterval)
				if backend.synth == 0 {
					t.Fatal("speech ended before its startup grace")
				}
				fixture.tick(speechStartGrace)
			case "maximum duration":
				driver.speaking = true
				fixture.tick(speechPollMax)
			}
			if completions != 1 || backend.synth != 0 || backend.cancelPoll != nil || len(driver.released) != 1 {
				t.Fatalf("incomplete cleanup: callbacks=%d synth=%d releases=%v", completions, backend.synth, driver.released)
			}
			pollCount := len(fixture.polls)
			fixture.tick(speechPollInterval)
			backend.Stop()
			if completions != 1 || len(fixture.polls) != pollCount || len(driver.released) != 1 {
				t.Fatal("a finished poll restarted speech or released an object twice")
			}
		})
	}
}

func TestAVSpeechStopAndReplacement(t *testing.T) {
	fixture := newAVSpeechFixture(t)
	backend, driver := fixture.backend, fixture.driver
	completions := 0
	done := func() { completions++ }
	if err := backend.Speak("one", done); err != nil {
		t.Fatal(err)
	}
	stale := fixture.polls[0]
	if err := backend.Speak("two", done); err != nil {
		t.Fatal(err)
	}
	if !stale.canceled || len(driver.released) != 1 || driver.released[0] != 1 || backend.synth != 2 {
		t.Fatal("replacement did not cancel polling and release the old synthesizer")
	}
	stale.callback()
	if driver.polled != 0 || backend.synth != 2 || completions != 0 {
		t.Fatal("a queued stale poll touched the new utterance")
	}
	pending := fixture.polls[len(fixture.polls)-1]
	backend.Stop()
	backend.Stop()
	pending.callback()
	if !pending.canceled || backend.synth != 0 || backend.cancelPoll != nil || driver.polled != 0 || len(driver.released) != 2 || completions != 0 {
		t.Fatal("stop left a timer, synthesizer or completion alive")
	}
}

func TestAVSpeechCreationFailureFallsBack(t *testing.T) {
	fixture := newAVSpeechFixture(t)
	fixture.driver.startErr = errors.New("creation failed")
	fallback := &fakeSpeechBackend{}
	backend := &fallbackSpeech{primary: fixture.backend, fallback: fallback}
	if err := backend.Speak("hello", nil); err != nil {
		t.Fatal(err)
	}
	if len(fixture.polls) != 0 || fixture.backend.synth != 0 || len(fallback.speaks) != 1 {
		t.Fatal("failed native creation left work alive or skipped the fallback")
	}
}

func TestSaySpeechStandardInputAndReaping(t *testing.T) {
	var output bytes.Buffer
	var child *exec.Cmd
	backend := &saySpeechBackend{command: func() *exec.Cmd {
		child = exec.Command("/bin/cat")
		child.Stdout = &output
		return child
	}}
	t.Cleanup(backend.Stop)
	done := make(chan struct{})
	text := "-v unexpected; $(false) `false`\n原文"
	if err := backend.Speak(text, func() { close(done) }); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("silent stdin fixture did not exit")
	}
	if output.String() != text || len(child.Args) != 1 || child.ProcessState == nil || !child.ProcessState.Success() {
		t.Fatal("source was interpreted as arguments or the child was not reaped")
	}
	backend.Stop()
}

func TestSaySpeechStopReapsChild(t *testing.T) {
	var child *exec.Cmd
	backend := &saySpeechBackend{command: func() *exec.Cmd {
		child = exec.Command("/bin/sleep", "30")
		return child
	}}
	t.Cleanup(backend.Stop)
	done := make(chan struct{}, 2)
	if err := backend.Speak("one", func() { done <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	first := child
	if err := backend.Speak("two", func() { done <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	if first.ProcessState == nil {
		t.Fatal("replacement did not reap the first child")
	}
	backend.Stop()
	backend.Stop()
	if child.ProcessState == nil || backend.cmd != nil || backend.exited != nil {
		t.Fatal("stop returned before the child was killed and reaped")
	}
	select {
	case <-done:
		t.Fatal("an interrupted child reported natural completion")
	default:
	}
}
