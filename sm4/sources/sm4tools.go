package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	flag "github.com/spf13/pflag"
	"github.com/tjfoc/gmsm/sm3"
	"github.com/tjfoc/gmsm/sm4"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"io"
	stdlog "log"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

type Encryptor interface {
	Encrypt([]byte) ([]byte, error)
	Decrypt([]byte) ([]byte, error)
}

const (
	tag        string = "sm4 encrypted"
	bufferSize int    = 4096
	iv         string = "sharework.cn"
)

func main() {

	// setup handler for SIG_TERM, SIG_KILL, etc.
	setupCloseHandler()

	// setup flags
	key := flag.StringP("key", "k", "", `key used for SM4 algorithm`)
	decrypt := flag.BoolP("decrypt", "d", false, "decrypt the data rather than encrypt it")
	force := flag.BoolP("force", "f", false,
		"force proceed for the files those already encrypted")
	test := flag.BoolP("test", "t", false,
		"test whether the file is valid to encrypt/decrypt")
	logLevel := flag.String("log-level", "info", "log level(fatal/error/warn/info/debug)")
	flag.Usage = func() {
		_, err := fmt.Fprintf(os.Stderr, `Command line tool for SM4 encryption/decryption
Usage: smtools [options...] <file>
Options`)
		if err != nil {
			return
		}
		_, err = fmt.Fprint(os.Stderr, "\n")
		if err != nil {
			return
		}
		flag.PrintDefaults()
	}
	flag.ErrHelp = errors.New("")
	flag.Parse()

	// create the log
	l, err := createLogger(*logLevel)
	if err != nil {
		stdlog.Fatalf("failed to create logger! \n%+v\n", err)
		return
	}
	log := l

	// determine the key
	if len(*key) != 16 {
		log.Fatal("key length is invalid, should be 16")
		return
	}

	// pb: prefix bytes is the hash value of the tag and the key
	hash := sm3.New()
	hash.Write([]byte(tag))
	hash.Write([]byte(*key))
	pb := []byte(hex.EncodeToString(hash.Sum(nil)))
	log.Debugf("desired prefix is %x\n", pb)

	r := bufio.NewReaderSize(os.Stdin, bufferSize) // r: reader
	w := bufio.NewWriter(os.Stdout)                // w: writer
	var b, o []byte                                // b: input buffer, o: output buffer
	var rl int                                     // rl: length of bytes read
	if *decrypt && !*test {                        // read and skip the prefix for decryption
		b = make([]byte, len(pb), len(pb))
		rl, err = r.Read(b)
		if err != nil && err != io.EOF {
			log.Fatalw("failed to read",
				"error", err)
			return
		}
	} else {
		rl = len(pb) // rl is always equal the length of pb in peeking mode
		b, err = r.Peek(rl)
		if err != nil && err != bufio.ErrBufferFull {
			log.Fatalw("failed to read",
				"error", err)
			return
		}
	}

	if rl == len(pb) && bytes.Equal(pb, b) {
		if *decrypt { // valid to decrypt
			if *test {
				return
			}
		} else { // invalid to encrypt
			if *test || !*force {
				log.Fatal("file had already been encrypted")
				return
			}
		}
	} else {
		if *decrypt { // invalid to decrypt
			log.Fatal("invalid key")
			return
		} else {
			if *test { // valid to encrypt
				return
			}
		}
	}

	b = make([]byte, bufferSize, bufferSize)
	err = sm4.SetIV([]byte(iv)) // set the IV value
	if err != nil {
		log.Fatalf("failed to set iv:\n%+v\n", err)
		return
	}

	hk := []byte(*key)
	for {
		rl, err = r.Read(b)
		if err != nil && err != io.EOF {
			log.Fatalf("failed to read:\n%+v\n", err)
			return
		}
		log.Debugf("input %x\n", b[:rl])
		if *decrypt {
			o, err = sm4.Sm4Ecb(hk, b[:rl], false)
			if err != nil {
				log.Fatalf("failed to decrypt:\n%+v\n", err)
				return
			}
		} else {
			o, err = sm4.Sm4Ecb(hk, b[rl:], true)
			if err != nil {
				log.Fatalf("failed to encrypt:\n%+v\n", err)
				return
			}
		}
		log.Debugf("output %x\n", o)
		_, err = w.Write(o)
		if err != nil {
			log.Fatalf("failed to write:\n%+v\n", err)
			return
		}
	}
}

func createLogger(level string) (logger *zap.SugaredLogger, err error) {

	var lvl zapcore.Level

	// parse log level
	switch strings.ToLower(level) {
	case "debug":
		lvl = zapcore.DebugLevel
	case "info":
		lvl = zapcore.InfoLevel
	case "warn":
		lvl = zapcore.WarnLevel
	case "error":
		lvl = zapcore.ErrorLevel
	case "fatal":
		lvl = zapcore.FatalLevel
	default:
		return nil, errors.New("invalid log level")
	}

	// log level-handlers

	l := zap.LevelEnablerFunc(func(zpl zapcore.Level) bool {
		return zpl >= lvl && zpl < zapcore.ErrorLevel
	})
	el := zap.LevelEnablerFunc(func(zpl zapcore.Level) bool {
		return zpl >= zapcore.ErrorLevel
	})

	// For clients implement zapcore.WriteSyncer and are safe for concurrent use,
	// Use zapcore.AddSync for client those are safe for concurrent use, and
	// zapcore.Lock for client that are not concurrency safe

	c := zapcore.Lock(os.Stdout)
	e := zapcore.Lock(os.Stderr)

	// define the encoder
	// je := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	ce := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())

	// error output to the stderr and others output to stdout.
	core := zapcore.NewTee(
		zapcore.NewCore(ce, e, el),
		zapcore.NewCore(ce, c, l),
	)

	return zap.New(core).Sugar(), nil
}

func cleanUp() {

}

func setupCloseHandler() {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGKILL)
	go func() {
		<-c
		cleanUp()
		os.Exit(0)
	}()
}
