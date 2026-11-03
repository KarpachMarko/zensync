package app

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestTransferSamplesAndBoundedOutput(t *testing.T) {
	a := &App{}
	output := &transferOutput{app: a, job: "job"}
	now := time.Now()
	// Resumed bytes are not counted as fresh throughput.
	output.event([]byte(`ZENSYNC_TRANSFER {"phase":"download","downloaded":500,"total":1000}`), now)
	p := a.transfer("job", "downloading")
	if p == nil || p.Percent == nil || *p.Percent != 50 || p.BytesPerSecond != nil {
		t.Fatal(p)
	}
	output.event([]byte(`ZENSYNC_TRANSFER {"phase":"download","downloaded":750,"total":1000}`), now.Add(time.Second))
	p = a.transfer("job", "downloading")
	if *p.Percent != 75 || p.BytesPerSecond == nil || *p.BytesPerSecond != 250 {
		t.Fatal(p)
	}
	// A restarted transfer changes the baseline, rather than showing negative speed.
	output.event([]byte(`ZENSYNC_TRANSFER {"phase":"download","downloaded":10,"total":0}`), now.Add(2*time.Second))
	p = a.transfer("job", "downloading")
	if p.Percent != nil || p.BytesPerSecond != nil || p.DownloadedBytes != 10 {
		t.Fatal(p)
	}
	for _, bad := range []string{
		`human output containing cookie=private`,
		`ZENSYNC_TRANSFER {"phase":"download","downloaded":-1,"total":100}`,
		`ZENSYNC_TRANSFER {"phase":"download","downloaded":101,"total":100}`,
		`ZENSYNC_TRANSFER {"phase":"download","downloaded":1,"total":999999999999}`,
		`ZENSYNC_TRANSFER {"phase":"download","downloaded":1.5,"total":100}`,
		`ZENSYNC_TRANSFER {"phase":"secret","downloaded":0,"total":0}`,
		`ZENSYNC_TRANSFER malformed`,
	} {
		output.Write([]byte(bad + "\n"))
	}
	if p = a.transfer("job", "downloading"); p.DownloadedBytes != 10 {
		t.Fatal("Untrusted output changed telemetry", p)
	}
	output.Write([]byte(strings.Repeat("x", 2000) + "\nZENSYNC_TRANS"))
	output.Write([]byte("FER {\"phase\":\"download\",\"downloaded\":50,\"total\":100}\n"))
	if p = a.transfer("job", "downloading"); p == nil || *p.Percent != 50 {
		t.Fatal("Stream chunk handling", p)
	}
	if p = a.transfer("job", "available"); p != nil {
		t.Fatal("Transfer exposed after completion")
	}
	a.clearTransfer("job")
	if p = a.transfer("job", "downloading"); p != nil {
		t.Fatal("Old attempt retained")
	}
	output.event([]byte(`ZENSYNC_TRANSFER {"phase":"download","downloaded":75,"total":100}`), now.Add(-10*time.Second))
	if p = a.transfer("job", "downloading"); p.BytesPerSecond == nil || *p.BytesPerSecond != 0 {
		t.Fatal("Stalled transfer retained speed", p)
	}
}

func TestTransferAPIIsolationAndPhaseReset(t *testing.T) {
	a, admin, member := albumFixture(t)
	var owner string
	if e := a.DB.QueryRow("SELECT id FROM users WHERE role='admin'").Scan(&owner); e != nil {
		t.Fatal(e)
	}
	_, e := a.DB.Exec("INSERT INTO requests(id,owner,idem,target,title,artist,upgrade,quality,decision) VALUES('transfer-request',?,'transfer-test','deezer:track:1','Private download','Artist',0,'[]','approved')", owner)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = a.DB.Exec("INSERT INTO jobs(id,request_id,state,stage) VALUES('transfer-job','transfer-request','downloading','Acquiring audio')"); e != nil {
		t.Fatal(e)
	}
	stream := &transferOutput{app: a, job: "transfer-job"}
	stream.Write([]byte("ZENSYNC_TRANSFER {\"phase\":\"download\",\"downloaded\":50,\"total\":100}\n"))
	for _, endpoint := range []string{"/api/v2/activity", "/api/v2/requests"} {
		w := admin.call("GET", endpoint, "", "")
		expect(t, w, 200)
		if !strings.Contains(w.Body.String(), `"percent":50`) {
			t.Fatal("Transfer missing from API", endpoint, w.Body)
		}
		w = member.call("GET", endpoint, "", "")
		expect(t, w, 200)
		if strings.Contains(w.Body.String(), "Private download") || strings.Contains(w.Body.String(), "transfer-job") {
			t.Fatal("Private transfer leaked", endpoint, w.Body)
		}
	}

	// Album track progress uses the same authorized child job metrics.
	for _, q := range []string{
		"INSERT INTO requests(id,owner,idem,target,title,artist,upgrade,quality,decision) VALUES('album-transfer','" + owner + "','album-transfer','deezer:album:42','Album','Artist',0,'[]','approved')",
		"INSERT INTO album_requests(request_id,snapshot) VALUES('album-transfer','{\"tracks\":[]}')",
		"INSERT INTO request_children(parent_id,child_id,position) VALUES('album-transfer','transfer-request',0)",
	} {
		if _, e = a.DB.Exec(q); e != nil {
			t.Fatal(e)
		}
	}
	items, e := a.albumItems("album-transfer")
	if e != nil || len(items) != 1 || items[0].Transfer == nil || *items[0].Transfer.Percent != 50 {
		t.Fatal("Album track telemetry missing", items, e)
	}
	stream.Write([]byte("ZENSYNC_TRANSFER {\"phase\":\"tagging\",\"downloaded\":0,\"total\":0}\n"))
	if a.transfer("transfer-job", "downloading") != nil {
		t.Fatal("Byte progress retained while tagging")
	}
	var stage string
	a.DB.QueryRow("SELECT stage FROM jobs WHERE id='transfer-job'").Scan(&stage)
	if stage != "Adding audio tags and artwork" {
		t.Fatal(stage)
	}
	stream.Write([]byte("ZENSYNC_TRANSFER {\"phase\":\"download\",\"downloaded\":50,\"total\":100}\n"))
	a.jobState("transfer-job", "validating", "Verifying audio")
	if a.transfer("transfer-job", "downloading") != nil {
		t.Fatal("Progress retained after transfer")
	}
}

// Exercise exec.Cmd's stdout copier, as used for both real and stock downloaders.
func TestDownloaderProgressSubprocess(t *testing.T) {
	if os.Getenv("ZENSYNC_TRANSFER_TEST_CHILD") == "1" {
		fmt.Println("human message with a private URL")
		fmt.Println(`ZENSYNC_TRANSFER {"phase":"download","downloaded":128,"total":256}`)
		os.Exit(0)
	}
	a := &App{}
	cmd := exec.Command(os.Args[0], "-test.run=^TestDownloaderProgressSubprocess$")
	cmd.Env = append(os.Environ(), "ZENSYNC_TRANSFER_TEST_CHILD=1")
	cmd.Stdout = &transferOutput{app: a, job: "child"}
	if e := cmd.Run(); e != nil {
		t.Fatal(e)
	}
	p := a.transfer("child", "downloading")
	if p == nil || p.Percent == nil || *p.Percent != 50 {
		t.Fatal(p)
	}
	b, _ := json.Marshal(p)
	if strings.Contains(string(b), "private") {
		t.Fatal("Raw child output leaked")
	}
}
