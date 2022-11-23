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

const (
	tag        string = "sm4 encrypted"
	bufferSize int    = 4096
	iv         string = "sharework.cn"
)

var (
	log zap.SugaredLogger
)

func main() {

	// setup flags
	key := flag.StringP("key", "k", "", `key used for SM4 algorithm`)
	decrypt := flag.BoolP("decrypt", "d", false, "decrypt the data rather than encrypt it")
	force := flag.BoolP("force", "f", false,
		"force proceed for the files those already encrypted")
	test := flag.BoolP("test", "t", false,
		"test whether the file is valid to encrypt/decrypt")
	logLevel := flag.String("log-level", "info", "log level(fatal/error/warn/info/debug)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Command line tool for SM4 encryption/decryption
Usage: smtools [options...] <file>
Options`)
		fmt.Fprint(os.Stderr, "\n")
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
		log.Fatal("key length error!(should be 16)")
		return
	}

	// pb: prefix bytes is the hash value of the tag and the key
	hash := sm3.New()
	hash.Write([]byte(tag))
	hash.Write([]byte(*key))
	pb := []byte(hex.EncodeToString(hash.Sum(nil)))

	r := bufio.NewReaderSize(os.Stdin, bufferSize) // r: reader
	w := bufio.NewWriter(os.Stdout)                // w: writer
	var b, o []byte                                // b: input buffer, o: output buffer
	var rl int                                     // rl: length of bytes read
	if *decrypt && !*test {                        // read and skip the prefix for decryption
		b = make([]byte, len(pb), len(pb))
		rl, err = r.Read(b)
		if err != nil && err != io.EOF {
			log.Errorw("failed to read",
				"error", err)
			return
		}
	} else {
		rl = len(pb) // rl is always equal the length of pb in peeking mode
		b, err = r.Peek(rl)
		if err != nil && err != bufio.ErrBufferFull {
			log.Errorw("failed to read",
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
	sm4.SetIV([]byte(iv)) // set the IV value
	hk := []byte(*key)
	ecbMsg, err := sm4.Sm4Ecb(hk, data, true) //sm4Ecb模式pksc7填充加密
	if err != nil {
		log.Errorf("sm4 enc error:\n%+v\n", err)
		return
	}
	log.Debugf("ecbMsg = %x\n", ecbMsg)
	ecbDec, err := sm4.Sm4Ecb(hk, ecbMsg, false) //sm4Ecb模式pksc7填充解密
	if err != nil {
		log.Errorf("sm4 dec error:%s", err)
		return
	}
	if !*decrypt {
		w.Write(pb)
	}
	for {
		rl, err = r.Read(b)
		if err != nil && err != io.EOF {
			log.Errorw("failed to read",
				"error", err)
			return
		}
		o, err := sm4.Sm4Ecb(hk, b[rl:], true)
		if err != nil {
			log.Errorf("sm4 enc error:\n%+v\n", err)
			return
		}
		w.Write(o)
	}
	// pipe line
	fi, err := os.Stdin.Stat()
	if err != nil {
		log.Fatalf("Failed to get state of stdin!\n%+v\n", err)
		return
	}
	if (fi.Mode() & os.ModeNamedPipe) != os.ModeNamedPipe {
		log.Fatalf("There's no input from named pipe!\n%+v\n", err)
		return
	}

	inPos := 0
	outPos := 0

	if !stdin {
		if len(fname) == 0 {
			log.Fatal("Input file is not provided!")
			return
		}
		// wrap a reader against a normal file
		_, err := os.Stat(fname)
		if err != nil {
			log.Fatalw("File not found!",
				"file", fname)
			return
		}
		fn, err := os.OpenFile(fname, os.O_RDONLY, 0666)
		if err != nil {
			log.Errorw("Can not open file!",
				"file", fname,
				"error", err)
			return
		}
		defer fn.Close()
		r = bufio.NewReaderSize(fn, bufferSize)
	}
	if len(fname) == 0 {
		// pipe line
		fi, err := os.Stdin.Stat()
		if err != nil {
			log.Errorf("Failed to get state of stdin!\n%+v\n", err)
			return
		}
		if (fi.Mode() & os.ModeNamedPipe) != os.ModeNamedPipe {
			log.Errorf("There's no input from named pipe!\n%+v\n", err)
			return
		}
		r = bufio.NewReaderSize(os.Stdin, bufferSize)
	} else {
		// normal file
		_, err := os.Stat(fname)
		if err != nil {
			log.Errorw("File not found!",
				"file", fname)
			return
		}
		fn, err := os.OpenFile(fname, os.O_RDONLY, 0666)
		if err != nil {
			log.Errorw("Can not open file!",
				"file", fname,
				"error", err)
			return
		}
		defer fn.Close()
		r = bufio.NewReaderSize(fn, bufferSize)
	}

	if len(tag) > 0 {
		tb, err := r.Peek(len(tag))
		if err != nil && err != io.EOF {
			log.Errorw("Can not read file!",
				"file", fname,
				"error", err)
			return
		}

		s := string(tb)
		encrypted := strings.Compare(s, tag) == 0

		if encrypted {
			if !decrypt {
				log.Warnw("File already encrypted!",
					"file", fname)
				return
			}
		} else {
			if decrypt {
				log.Errorw(
					"File is not encrypted!",
					"file", fname,
				)
			}
		}
	}

	hk := []byte(*key)

	log.Debugf("key = %v\n", hk)
	data := []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10}
	log.Debugf("data = %x\n", data)
	sm4.SetIV([]byte(iv))                     //设置SM4算法实现的IV值,不设置则使用默认值
	ecbMsg, err := sm4.Sm4Ecb(hk, data, true) //sm4Ecb模式pksc7填充加密
	if err != nil {
		log.Errorf("sm4 enc error:\n%+v\n", err)
		return
	}
	log.Debugf("ecbMsg = %x\n", ecbMsg)
	ecbDec, err := sm4.Sm4Ecb(hk, ecbMsg, false) //sm4Ecb模式pksc7填充解密
	if err != nil {
		log.Errorf("sm4 dec error:%s", err)
		return
	}
	log.Debugf("ecbDec = %x\n", ecbDec)
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
	// zapcore.Lock for client that are not concurrency safe)

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
