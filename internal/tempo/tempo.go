package tempo

import (
    "bytes"
    "context"
    _ "embed"
    "encoding/json"
    "errors"
    "fmt"
    "os/exec"
    "strings"
    "time"

    "github.com/ichiken26/cusic/internal/analysis"
)

//go:embed worker.py
var worker string

type Candidate struct {
    BPM float64 `json:"bpm"`
    Score float64 `json:"score"`
}
type Report struct {
    Version int `json:"version"`
    Engine string `json:"engine"`
    EstimatedBPM float64 `json:"estimatedBpm"`
    Confidence float64 `json:"confidence"`
    TempoCandidates []Candidate `json:"tempoCandidates"`
    LocalBPM []float64 `json:"localBpm"`
    StableLocalTempo bool `json:"stableLocalTempo"`
    DurationSeconds float64 `json:"durationSeconds"`
    BeatTimesSeconds []float64 `json:"beatTimesSeconds"`
    BeatCoverage float64 `json:"beatCoverage"`
    Limitations string `json:"limitations"`
}

func Analyze(ctx context.Context, path string) (Report, error) {
    var result Report
    if strings.TrimSpace(path)=="" { return result, errors.New("tempo source WAV required") }
    python, prefix, err := analysis.Python()
    if err!=nil {return result, err}
    data, err := json.Marshal(map[string]string{"path":path})
    if err!=nil {return result, err}
    timeout, cancel := context.WithTimeout(ctx, 3*time.Minute)
    defer cancel()
    args := append(prefix, "-c", worker)
    cmd := exec.CommandContext(timeout, python, args...)
    cmd.Stdin = bytes.NewReader(data)
    var stdout, stderr bytes.Buffer
    cmd.Stdout=&stdout; cmd.Stderr=&stderr
    if err=cmd.Run(); err!=nil {
        if timeout.Err()!=nil {return result, fmt.Errorf("tempo analysis timed out: %w", timeout.Err())}
        detail:=strings.TrimSpace(stderr.String())
        if len(detail)>2000 {detail=detail[:2000]}
        if detail=="" {detail=err.Error()}
        return result, fmt.Errorf("tempo analysis: %s", detail)
    }
    if stdout.Len()>2<<20 {return result, errors.New("oversized tempo analysis output")}
    if err=json.Unmarshal(stdout.Bytes(),&result); err!=nil {return result, fmt.Errorf("tempo worker JSON: %w",err)}
    if result.Version!=1 || result.Engine!="numpy-scipy-onset" || result.EstimatedBPM<20 || result.EstimatedBPM>400 ||
        result.Confidence<0 || result.Confidence>1 || result.DurationSeconds<=0 {
        return Report{}, errors.New("invalid tempo worker report")
    }
    return result,nil
}
