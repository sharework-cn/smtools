package main

import (
	"bufio"
	"bytes"
	"errors"

	"fmt"
	"io"
	stdlog "log"
	"os"
	"path/filepath"
	"strings"

	"github.com/dimchansky/utfbom"
	"github.com/mitchellh/go-homedir"
	flag "github.com/spf13/pflag"
	"github.com/tjfoc/gmsm/sm4"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/transform"
)

const (
	tag               string = "smtools encrypted"
	bufferSize        int    = 4096
	envKey            string = "SM4_KEY"
	configPath        string = "/conf"
	securedConfigPath string = "/secrets"
	folderKey         string = "secrets"
	fileKey           string = "sm4key"
)

var (
	log zap.SugaredLogger
)

func main() {

	var (
		decrypt  bool   // flag - decrypt
		key      string // flag - key
		logLevel string // flag - log-level
		output   string // flag - output
		stdin    bool   // flag - stdin
		tag      string // flag - tag
		fname    string // argument 1 - input file name
	)

	// cli arguments processing
	flag.StringVarP(&key, "key", "k", "", `Key used for SM4 algorithm`)
	flag.BoolVarP(&decrypt, "decrypt", "d", false, "Decrypt the data rather than encrypt it")
	flag.BoolVar(&stdin, "stdin", false, "Read data from stdin")
	flag.StringVarP(&output, "output", "o", "", "Output file name, default to stdout")
	flag.StringVar(&tag, "tag", "", "A leading tag inserted into the encrypted file")
	flag.StringVar(&logLevel, "log-level", "info", "Log level(fatal/error/warn/info/debug)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Command line tool for SM4 encryption/decryption
Usage: smtools [options...] <file>
Options`)
		fmt.Fprint(os.Stderr, "\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	projectPath, _ := os.Getwd()
	fmt.Fprint(os.Stdout, projectPath)
	var cp string
	var scp string
	if !filepath.IsAbs(configPath) {
		cp = filepath.Join(projectPath, configPath)
	} else {
		cp = configPath
	}
	if !filepath.IsAbs(securedConfigPath) {
		scp = filepath.Join(projectPath, securedConfigPath)
	} else {
		scp = securedConfigPath
	}
	fmt.Fprintf(os.Stdout, "config path is %s, %s", cp, scp)

	// create log
	l, err := createLogger(logLevel)
	if err != nil {
		stdlog.Fatalf("Failed to create logger! \n%s", err)
		return
	}
	log := l

	// determine the key
	if len(key) == 0 {
		key = os.Getenv(envKey)
	}
	if len(key) == 0 {
		// wrap a reader against a normal file
		h, err := homedir.Dir()
		if err != nil {
			log.Fatal("Failed to get home path!")
			return
		}
		kf := filepath.Join(h, folderKey, fileKey)
		_, err = os.Stat(kf)
		if err != nil {
			log.Fatalf("Key is not specified!\n%s",
				err)
			return
		}
		fn, err := os.OpenFile(kf, os.O_RDONLY, 0666)
		if err != nil {
			log.Fatalw("Can not open file!",
				"file", fileKey,
				"error", err)
			return
		}
		defer fn.Close()
		sc := bufio.NewScanner(fn)
		sc.Scan()
		key = string(sc.Bytes())
		ec, n, certain := charset.DetermineEncoding(sc.Bytes(), "text/html")
		if !certain {
			log.Fatal("Can not determine the encoding for key file!")
			return
		}

		r := bufio.NewReaderSize(transform.NewReader(utfbom.SkipOnly(bytes.NewReader(sc.Bytes())),
			ec.NewDecoder()), bufferSize)
		s, p, err := r.ReadLine()
		if err != nil {
			log.Fatalw("Can not transform key file!",
				"encoding", n)
			return
		}
		if p {
			log.Fatal("The key file is too large!")
			return
		}
		key = string(s)
	}
	if len(key) != 16 {
		log.Fatal("key length error!(should be 16)")
		return
	}

	var r *bufio.Reader // reader to get the data

	// wrap the reader:
	// 1. from `stdin` if flag set
	// 2. from the file which name provided in stdin if `fname` not provided
	if stdin || len(fname) == 0 {
		// pipe line
		fi, err := os.Stdin.Stat()
		if err != nil {
			log.Fatalf("Failed to get state of stdin!\n%s", err)
			return
		}
		if (fi.Mode() & os.ModeNamedPipe) != os.ModeNamedPipe {
			log.Fatalf("There's no input from named pipe!\n%s", err)
			return
		}
		r = bufio.NewReaderSize(os.Stdin, bufferSize)
		// If user want read from file definitely, we should get the file name
		// from stdin
		if len(fname) == 0 && !stdin {
			s := bufio.NewScanner(os.Stdin)
			s.Scan()
			fname = s.Text()
		}
	}

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
			log.Errorf("Failed to get state of stdin!\n%s", err)
			return
		}
		if (fi.Mode() & os.ModeNamedPipe) != os.ModeNamedPipe {
			log.Errorf("There's no input from named pipe!\n%s", err)
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

	hk := []byte(key)

	log.Debugf("key = %v\n", hk)
	data := []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10}
	log.Debugf("data = %x\n", data)
	iv := []byte("0000000000000000")
	sm4.SetIV(iv)                             //设置SM4算法实现的IV值,不设置则使用默认值
	ecbMsg, err := sm4.Sm4Ecb(hk, data, true) //sm4Ecb模式pksc7填充加密
	if err != nil {
		log.Errorf("sm4 enc error:%s", err)
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
		return nil, errors.New("Invalid log level!")
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
