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

// avSpeechBackend keeps one synthesizer alive from creation until the
// utterance ends or is stopped. A local object would be collected while
// AppKit is still speaking. Creation, speaking, release and the isSpeaking
// poll all run on the main thread; the poll only waits on a worker.
type avSpeechBackend struct {
	mu         sync.Mutex
	synth      objc.ID
	generation uint64
}

func (b *avSpeechBackend) Speak(text string, done func()) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errSpeechUnavailable
	}
	speechOnce.Do(loadSpeechClasses)
	if speechLoadErr != nil {
		return speechLoadErr
	}
	b.stop()
	var started bool
	var startErr error
	mygo.RunOnMain(func() {
		pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(speechSelector.newPool)
		defer func() {
			if pool != 0 {
				pool.Send(speechSelector.drain)
			}
		}()
		synth := objc.ID(speechClasses.synth).Send(speechSelector.alloc).Send(speechSelector.init)
		if synth == 0 {
			startErr = errSpeechUnavailable
			return
		}
		utterance := objc.ID(speechClasses.utterance).Send(speechSelector.utteranceWithString, speechString(text))
		if utterance == 0 {
			synth.Send(speechSelector.release)
			startErr = errSpeechUnavailable
			return
		}
		if voice := objc.ID(speechClasses.voice).Send(speechSelector.voiceWithLanguage, speechString(speechLanguage(text))); voice != 0 {
			utterance.Send(speechSelector.setVoice, voice)
		}
		synth.Send(speechSelector.speakUtterance, utterance)
		b.mu.Lock()
		b.generation++
		generation := b.generation
		b.synth = synth
		b.mu.Unlock()
		started = true
		go b.watch(generation, done)
	})
	if startErr != nil {
		return startErr
	}
	if !started {
		return errSpeechUnavailable
	}
	return nil
}

// watch polls isSpeaking on the main thread. The synthesizer is released only
// after speech ends, so a second click or a closed panel stops it early while
// a finished utterance still clears the UI marker through done.
func (b *avSpeechBackend) watch(generation uint64, done func()) {
	deadline := time.Now().Add(speechPollMax)
	wasSpeaking := false
	for time.Now().Before(deadline) {
		time.Sleep(speechPollInterval)
		var speaking, active bool
		// Hold the lock across the main-thread call: Stop may clear and
		// release the synthesizer while this poll is queued.
		mygo.RunOnMain(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			active = b.generation == generation && b.synth != 0
			if active {
				speaking = b.synth.Send(speechSelector.isSpeaking) != 0
			}
		})
		if !active {
			return
		}
		if speaking {
			wasSpeaking = true
		} else if wasSpeaking {
			break
		}
	}
	b.mu.Lock()
	var releaseID objc.ID
	if b.generation == generation && b.synth != 0 {
		releaseID = b.synth
		b.synth = 0
	}
	b.mu.Unlock()
	if releaseID == 0 {
		return
	}
	mygo.RunOnMain(func() { releaseID.Send(speechSelector.release) })
	if done != nil {
		done()
	}
}

func (b *avSpeechBackend) stop() {
	b.mu.Lock()
	b.generation++
	synth := b.synth
	b.synth = 0
	b.mu.Unlock()
	if synth == 0 {
		return
	}
	mygo.RunOnMain(func() {
		synth.Send(speechSelector.stopSpeaking, uintptr(speechBoundaryNow))
		synth.Send(speechSelector.release)
	})
}

func (b *avSpeechBackend) Stop() { b.stop() }

// saySpeechBackend is the fallback. Text goes through standard input, never
// through a shell or a command line that could be parsed as options; Stop
// kills the child process. A natural exit reports done, a killed one does not.
type saySpeechBackend struct {
	mu         sync.Mutex
	cmd        *exec.Cmd
	generation uint64
}

func (b *saySpeechBackend) Speak(text string, done func()) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errSpeechUnavailable
	}
	b.Stop()
	cmd := exec.Command("/usr/bin/say", "-f", "-")
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Start(); err != nil {
		return err
	}
	b.mu.Lock()
	b.generation++
	generation := b.generation
	b.cmd = cmd
	b.mu.Unlock()
	go func() {
		_ = cmd.Wait()
		b.mu.Lock()
		active := b.generation == generation && b.cmd == cmd
		if active {
			b.cmd = nil
		}
		b.mu.Unlock()
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
	b.cmd = nil
	b.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// newSystemSpeech installs the real engine only in the running application.
func newSystemSpeech() speechEngine {
	return &fallbackSpeech{primary: &avSpeechBackend{}, fallback: &saySpeechBackend{}}
}
