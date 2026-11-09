//go:build darwin

package main

import (
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

// AVSpeechSynthesizer lives in AVFAudio. dlopen loads the ObjC classes before
// objc_getClass: the review experiment found a null class when the framework
// was not loaded. CoreFoundation/Foundation supply NSString and the pool.
const (
	avfaudioLibrary     = "/System/Library/Frameworks/AVFAudio.framework/AVFAudio"
	foundationLibrary   = "/System/Library/Frameworks/Foundation.framework/Foundation"
	speechPollInterval  = 100 * time.Millisecond
	speechStartGrace    = time.Second
	speechPollMax       = 2 * time.Minute
	speechBoundaryNow   = 0 // AVSpeechBoundaryImmediate
	speechVoiceLanguage = "en-US"
)

var (
	speechOnce     sync.Once
	speechLoadErr  error
	speechClasses  struct{ synth, utterance, voice objc.ID }
	speechSelector struct {
		alloc, init, utf8String, utteranceWithString, setVoice,
		voiceWithLanguage, speakUtterance, stopSpeaking, isSpeaking,
		release, newPool, drain objc.SEL
	}
)

func loadSpeechClasses() {
	// RTLD_GLOBAL registers the classes with the ObjC runtime, which is what
	// the review experiment needed before objc_getClass returned non-null.
	if _, err := purego.Dlopen(foundationLibrary, purego.RTLD_LAZY|purego.RTLD_GLOBAL); err != nil {
		speechLoadErr = errSpeechUnavailable
		return
	}
	if _, err := purego.Dlopen(avfaudioLibrary, purego.RTLD_LAZY|purego.RTLD_GLOBAL); err != nil {
		speechLoadErr = errSpeechUnavailable
		return
	}
	speechClasses.synth = objc.ID(objc.GetClass("AVSpeechSynthesizer"))
	speechClasses.utterance = objc.ID(objc.GetClass("AVSpeechUtterance"))
	speechClasses.voice = objc.ID(objc.GetClass("AVSpeechSynthesisVoice"))
	if speechClasses.synth == 0 || speechClasses.utterance == 0 || speechClasses.voice == 0 {
		speechLoadErr = errSpeechUnavailable
		return
	}
	speechSelector.alloc = objc.RegisterName("alloc")
	speechSelector.init = objc.RegisterName("init")
	speechSelector.utf8String = objc.RegisterName("stringWithUTF8String:")
	speechSelector.utteranceWithString = objc.RegisterName("utteranceWithString:")
	speechSelector.setVoice = objc.RegisterName("setVoice:")
	speechSelector.voiceWithLanguage = objc.RegisterName("voiceWithLanguage:")
	speechSelector.speakUtterance = objc.RegisterName("speakUtterance:")
	speechSelector.stopSpeaking = objc.RegisterName("stopSpeakingAtBoundary:")
	speechSelector.isSpeaking = objc.RegisterName("isSpeaking")
	speechSelector.release = objc.RegisterName("release")
	speechSelector.newPool = objc.RegisterName("new")
	speechSelector.drain = objc.RegisterName("drain")
}

// speechString bridges a Go string to an autoreleased NSString. purego passes
// the Go string as a C string; the explicit NUL matches the other darwin seams.
func speechString(value string) objc.ID {
	return objc.ID(objc.GetClass("NSString")).Send(speechSelector.utf8String, value+"\x00")
}

// speechLanguage picks the voice for the source text. English words stay on an
// English voice; Chinese, Japanese and Korean sources get their own language
// so reading the original never sounds like a different language.
func speechLanguage(text string) string {
	han, kana, hangul := false, false, false
	for _, r := range text {
		switch {
		case unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r):
			kana = true
		case unicode.Is(unicode.Hangul, r):
			hangul = true
		case unicode.Is(unicode.Han, r):
			han = true
		}
	}
	switch {
	case kana:
		return "ja-JP"
	case hangul:
		return "ko-KR"
	case han:
		return "zh-CN"
	default:
		return speechVoiceLanguage
	}
}

type avSpeechDriver interface {
	Start(string) (objc.ID, error)
	IsSpeaking(objc.ID) bool
	StopAndRelease(objc.ID)
}

type systemAVSpeech struct{}

func (systemAVSpeech) Start(text string) (objc.ID, error) {
	speechOnce.Do(loadSpeechClasses)
	if speechLoadErr != nil {
		return 0, speechLoadErr
	}
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(speechSelector.newPool)
	defer func() {
		if pool != 0 {
			pool.Send(speechSelector.drain)
		}
	}()
	synth := speechClasses.synth.Send(speechSelector.alloc).Send(speechSelector.init)
	if synth == 0 {
		return 0, errSpeechUnavailable
	}
	utterance := speechClasses.utterance.Send(speechSelector.utteranceWithString, speechString(text))
	if utterance == 0 {
		synth.Send(speechSelector.release)
		return 0, errSpeechUnavailable
	}
	if voice := speechClasses.voice.Send(speechSelector.voiceWithLanguage, speechString(speechLanguage(text))); voice != 0 {
		utterance.Send(speechSelector.setVoice, voice)
	}
	synth.Send(speechSelector.speakUtterance, utterance)
	return synth, nil
}

func (systemAVSpeech) IsSpeaking(synth objc.ID) bool {
	return objc.Send[bool](synth, speechSelector.isSpeaking)
}

func (systemAVSpeech) StopAndRelease(synth objc.ID) {
	synth.Send(speechSelector.stopSpeaking, uintptr(speechBoundaryNow))
	synth.Send(speechSelector.release)
}

type avSpeechBackend struct {
	driver      avSpeechDriver
	onMain      func(func())
	after       func(time.Duration, func()) func()
	now         func() time.Time
	synth       objc.ID
	generation  uint64
	cancelPoll  func()
	started     time.Time
	wasSpeaking bool
}

func (b *avSpeechBackend) Speak(text string, done func()) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errSpeechUnavailable
	}
	startErr := errSpeechUnavailable
	b.onMain(func() {
		b.stopOnMain()
		b.synth, startErr = b.driver.Start(text)
		if startErr != nil || b.synth == 0 {
			b.stopOnMain()
			startErr = errSpeechUnavailable
			return
		}
		b.started, b.wasSpeaking = b.now(), false
		b.schedulePoll(b.generation, done)
	})
	return startErr
}

func (b *avSpeechBackend) schedulePoll(generation uint64, done func()) {
	b.cancelPoll = b.after(speechPollInterval, func() {
		b.onMain(func() {
			if b.generation != generation || b.synth == 0 {
				return
			}
			b.cancelPoll = nil
			speaking := b.driver.IsSpeaking(b.synth)
			elapsed := b.now().Sub(b.started)
			if elapsed >= speechPollMax || !speaking && (b.wasSpeaking || elapsed >= speechStartGrace) {
				b.stopOnMain()
				if done != nil {
					done()
				}
				return
			}
			b.wasSpeaking = b.wasSpeaking || speaking
			b.schedulePoll(generation, done)
		})
	})
}

func (b *avSpeechBackend) stopOnMain() {
	b.generation++
	if b.cancelPoll != nil {
		b.cancelPoll()
		b.cancelPoll = nil
	}
	if b.synth != 0 {
		b.driver.StopAndRelease(b.synth)
		b.synth = 0
	}
}

func (b *avSpeechBackend) Stop() { b.onMain(b.stopOnMain) }

// saySpeechBackend is the fallback. Text goes through standard input, never
// through a shell or a command line that could be parsed as options; Stop
// kills the child process. A natural exit reports done, a killed one does not.
type saySpeechBackend struct {
	mu         sync.Mutex
	command    func() *exec.Cmd
	cmd        *exec.Cmd
	exited     chan struct{}
	generation uint64
}

func (b *saySpeechBackend) Speak(text string, done func()) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errSpeechUnavailable
	}
	b.Stop()
	cmd := b.command()
	cmd.Stdin = strings.NewReader(text)
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		return err
	}
	b.mu.Lock()
	b.generation++
	generation := b.generation
	b.cmd = cmd
	exited := make(chan struct{})
	b.exited = exited
	b.mu.Unlock()
	go func() {
		_ = cmd.Wait()
		b.mu.Lock()
		active := b.generation == generation && b.cmd == cmd
		if active {
			b.cmd = nil
			b.exited = nil
		}
		b.mu.Unlock()
		close(exited)
		if active && done != nil {
			done()
		}
	}()
	return nil
}

func (b *saySpeechBackend) Stop() {
	b.mu.Lock()
	b.generation++
	cmd := b.cmd
	exited := b.exited
	b.cmd = nil
	b.exited = nil
	b.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		<-exited
	}
}

// newSystemSpeech installs the real engine only in the running application.
func newSystemSpeech() speechEngine {
	return &fallbackSpeech{
		primary: &avSpeechBackend{
			driver: systemAVSpeech{}, onMain: mygo.RunOnMain, now: time.Now,
			after: func(delay time.Duration, fn func()) func() {
				timer := time.AfterFunc(delay, fn)
				return func() { timer.Stop() }
			},
		},
		fallback: &saySpeechBackend{command: func() *exec.Cmd {
			return exec.Command("/usr/bin/say", "-f", "-")
		}},
	}
}
