package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/leifjacky/ckb-gominer-demo/eaglesong"
	"github.com/sirupsen/logrus"
)

type StratumMiner struct {
	cfg *StratumMinerConfig

	target     *big.Int
	nonce1     uint64
	nonce2Size int
	job        atomic.Value
	cnt        int64

	writeMu sync.Mutex
	conn    net.Conn
}

type Job struct {
	sync.Mutex
	jobId   string
	powHash string
	nonce   uint64
}

func (j *Job) GetNextNonce(size int) uint64 {
	j.Lock()
	defer j.Unlock()
	n := j.nonce & (1<<((uint(size)-1)*8) - 1)
	j.nonce++
	return n
}

func NewMiner(cfg *StratumMinerConfig) *StratumMiner {
	return &StratumMiner{
		cfg:    cfg,
		target: new(big.Int).SetInt64(0),
	}
}

func (m *StratumMiner) Mine() {
	gracefulShutdownChannel := make(chan os.Signal)
	signal.Notify(gracefulShutdownChannel, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-gracefulShutdownChannel
		logrus.Warningf("receive shutdown signal")
		os.Exit(0)
	}()

	sumIntv := MustParseDuration(m.cfg.SumIntv)
	logrus.Infof("hashrate sum every %v", sumIntv)
	sumTicker := time.NewTicker(sumIntv)

	go m.start()
	for {
		select {
		case <-sumTicker.C:
			cnt := m.cnt
			m.cnt -= cnt
			logrus.Warningf("hashrates: %v", GetReadableHashRateString(float64(cnt/int64((sumIntv)/time.Second))))
		}
	}
}

func (m *StratumMiner) start() {
	th := m.cfg.Threads
	if th == 0 {
		th = runtime.NumCPU()
	}
	logrus.Infof("running with %v workers", th)
	for i := 0; i < th; i++ {
		go m.startWorker(i)
	}

	logrus.Infof("connect to %v", m.cfg.Url)
	conn, err := net.Dial("tcp", m.cfg.Url)
	if err != nil {
		logrus.Fatalf("failed to connect: %v", err)
	}
	m.conn = conn
	logrus.Infof("connected")

	buf := bufio.NewReader(conn)

	if err := m.request("mining.subscribe", []interface{}{"ckbminer-v1.0.0", nil}); err != nil {
		logrus.Fatalf("error subscribe: %v", err)
	}
	data, _, err := buf.ReadLine()
	if err != nil {
		logrus.Errorf("err reading: %v", err)
		return
	}
	logrus.Debugf("recv from pool: %v", string(data))
	if err := m.handleMesg(data, 1); err != nil {
		logrus.Errorf("err handle mesg: %v", err)
		return
	}
	logrus.Infof("subscribed")

	if err := m.request("mining.authorize", []string{m.cfg.Username, m.cfg.Password}); err != nil {
		logrus.Fatalf("error authorize: %v", err)
	}
	data, _, err = buf.ReadLine()
	if err != nil {
		logrus.Errorf("err reading: %v", err)
		return
	}
	logrus.Debugf("recv from pool: %v", string(data))
	if err := m.handleMesg(data, 2); err != nil {
		logrus.Errorf("err handle mesg: %v", err)
		return
	}
	logrus.Infof("authorized")

	for {
		data, _, err := buf.ReadLine()
		if err != nil {
			logrus.Errorf("err reading: %v", err)
			return
		}

		logrus.Debugf("recv from pool: %v", string(data))
		if err := m.handleMesg(data, 0); err != nil {
			logrus.Errorf("err handle mesg: %v", err)
			return
		}
	}
	logrus.Infof("disconnected")
}

func (m *StratumMiner) handleMesg(line []byte, flag int) error {
	var mesg PoolMesg
	if err := json.Unmarshal(line, &mesg); err != nil {
		return fmt.Errorf("can't decode: %v", err)
	}
	switch flag {
	case 1:
		if mesg.Error == nil {
			result := []interface{}{}
			if err := json.Unmarshal(*mesg.Result, &result); err != nil {
				return fmt.Errorf("can't decode result: %v", err)
			}
			m.nonce2Size = int(result[2].(float64))
			m.nonce1 = uint64(MustParseInt64(result[1].(string), 16) << (uint(m.nonce2Size) * 8))
		} else {
			info := []interface{}{}
			if err := json.Unmarshal(*mesg.Error, &info); err != nil {
				return fmt.Errorf("can't decode error: %v", err)
			}
			return fmt.Errorf("subscribe error. %v", info[1].(string))
		}
		return nil
	case 2:
		if mesg.Error != nil {
			info := []interface{}{}
			if err := json.Unmarshal(*mesg.Error, &info); err != nil {
				return fmt.Errorf("can't decode error: %v", err)
			}
			return fmt.Errorf("subscribe error. %v", info[1].(string))
		}
		return nil
	}
	switch mesg.Method {
	case "mining.set_target":
		params := []string{}
		if err := json.Unmarshal(*mesg.Params, &params); err != nil {
			return fmt.Errorf("can't decode params: %v", err)
		}
		if len(params) > 0 {
			target, ok := new(big.Int).SetString(params[0], 16)
			if !ok {
				return fmt.Errorf("invalid target")
			}
			m.target = target
			logrus.Infof("target set to: %064x", target)
		}
	case "mining.notify":
		params := []interface{}{}
		if err := json.Unmarshal(*mesg.Params, &params); err != nil {
			return fmt.Errorf("can't decode params: %v", err)
		}
		if len(params) < 2 {
			return fmt.Errorf("invalid params")
		}
		jobId, ok := params[0].(string)
		if !ok {
			return fmt.Errorf("invalid jobId")
		}
		powHash, ok := params[1].(string)
		_, err := hex.DecodeString(powHash)
		if !ok || err != nil {
			return fmt.Errorf("invalid powHash")
		}
		logrus.Infof("new job: %v - %v", jobId, powHash)
		m.job.Store(&Job{
			jobId:   jobId,
			powHash: powHash,
			nonce:   0,
		})
	default:
		result := false
		if err := json.Unmarshal(*mesg.Result, &result); err != nil {
			return fmt.Errorf("can't decode result: %v", err)
		}
		if result {
			logrus.Infof("share accepted")
		} else {
			info := []interface{}{}
			if err := json.Unmarshal(*mesg.Error, &info); err != nil {
				return fmt.Errorf("can't decode error: %v", err)
			}
			logrus.Infof("share rejected. %v", info[1].(string))
		}
	}
	return nil
}

type JsonRpcReq struct {
	Id     int64       `json:"id"`
	Method string      `json:"method"`
	Params interface{} `json:"params"`
}

type PoolMesg struct {
	Id     *json.RawMessage `json:"id"`
	Method string           `json:"method"`
	Result *json.RawMessage `json:"result"`
	Params *json.RawMessage `json:"params"`
	Error  *json.RawMessage `json:"error"`
}

func (m *StratumMiner) request(method string, params interface{}) error {
	return m.write(&JsonRpcReq{0, method, params})
}

var lineDelimiter = []byte("\n")

func (m *StratumMiner) write(message interface{}) error {
	b, err := json.Marshal(message)
	if err != nil {
		return err
	}

	m.writeMu.Lock()
	defer m.writeMu.Unlock()

	logrus.Debugf("write to pool: %v", string(b))
	if _, err := m.conn.Write(b); err != nil {
		return err
	}

	_, err = m.conn.Write(lineDelimiter)
	return err
}

func (m *StratumMiner) loadJob() *Job {
	job := m.job.Load()
	if job == nil {
		return nil
	}
	return job.(*Job)
}

func (m *StratumMiner) startWorker(i int) {
	for {
		job := m.loadJob()
		if job == nil {
			logrus.Warningf("#%d job not ready. sleep for 5s...", i)
			time.Sleep(5 * time.Second)
			continue
		}
		powhash := job.powHash
		nonce2 := job.GetNextNonce(m.nonce2Size)
		nonce := m.nonce1 + nonce2
		pattern := fmt.Sprintf("%%0%dx", m.nonce2Size*2)
		nonce2St := fmt.Sprintf(pattern, nonce2)
		b := append(UInt64BEToBytes(nonce), MustStringToHexBytes(powhash)...)
		hash := eaglesong.EaglesongHash(b)
		bInt := Hash2BigTarget(hash)
		if bInt.Cmp(m.target) <= 0 {
			logrus.Tracef("solve %x %064x", b, hash)
			logrus.Infof("share found: %v - %064x", nonce2St, bInt)
			go func() {
				if err := m.request("mining.submit", []interface{}{m.cfg.Username, job.jobId, nonce2St}); err != nil {
					logrus.Fatalf("error submit: %v", err)
				}
			}()
		}
		m.cnt++
	}
}
