// Package analysis bridges Go's MCP/CLI to an optional offline Python
// NumPy/SciPy worker. It sends file paths via JSON stdin, never shell commands.
// Python writes ONLY structured results on stdout and never edits the audio.
package analysis

import (
 "bytes"
 "context"
 _ "embed"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "os/exec"
 "runtime"
 "strings"
 "time"
)

//go:embed worker.py
var workerScript string

type File struct {
 Name string `json:"name"`
 Path string `json:"path"`
}
type Request struct {
 Files []File `json:"files"`
 MaxWindows int `json:"maxWindows"`
}
type Stem struct {
 Name string `json:"name"`
 SampleRate int `json:"sampleRate"`
 Channels int `json:"channels"`
 DurationSeconds float64 `json:"durationSeconds"`
 WindowsAnalyzed int `json:"windowsAnalyzed"`
 WindowSamples int `json:"windowSamples"`
 SampledPeakDBFS float64 `json:"sampledPeakDbfs"`
 SampledRmsDBFS float64 `json:"sampledRmsDbfs"`
 BandRmsDBFS map[string]float64 `json:"bandRmsDbfs"`
 SpectralCentroidHz float64 `json:"spectralCentroidHz"`
 SpectralRolloff95Hz float64 `json:"spectralRolloff95Hz"`
 DominantFrequenciesHz []float64 `json:"dominantFrequenciesHz"`
 StereoCorrelation *float64 `json:"stereoCorrelation"`
}
type Pair struct {
 Left string `json:"left"`
 Right string `json:"right"`
 BandSpectralSimilarity float64 `json:"bandSpectralSimilarity"`
}
type Report struct {
 Version int `json:"version"`
 Engine string `json:"engine"`
 Method string `json:"method"`
 Stems []Stem `json:"stems"`
 Pairs []Pair `json:"pairs"`
 Limitations string `json:"limitations"`
}

// candidatePythonAvailable checks the same dependency ranges as
// audio/requirements.txt. Probing candidates avoids picking a Python alias
// without NumPy while a different installed Python is already usable.
func candidatePythonAvailable(path string, prefix []string) bool {
 probe := "import sys,numpy,scipy; assert sys.version_info >= (3,10); assert 2 <= int(numpy.__version__.split('.')[0]) < 3; assert (1,13) <= tuple(int(v) for v in scipy.__version__.split('.')[:2]) < (2,0)"
 ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second)
 defer cancel()
 args:=append(append([]string{},prefix...),"-c",probe)
 cmd:=exec.CommandContext(ctx,path,args...)
 return cmd.Run()==nil
}

func Python() (string, []string, error) {
 if val:=os.Getenv("CUSIC_PYTHON");val!=""{
  path,err:=exec.LookPath(val)
  if err!=nil{return "",nil,fmt.Errorf("CUSIC_PYTHON=%s not found: %w",val,err)}
  return path,nil,nil
 }

 candidates:=[]string{"python3.12","python3.13","python3.11","python3.10","python3","python"}
 if runtime.GOOS=="windows"{candidates=[]string{"python","python3","python3.12","python3.13","python3.11","python3.10"}}
 // Prefer existing compatible packages to a stale private environment.
 for _,name:=range candidates{
  if path,err:=exec.LookPath(name);err==nil && candidatePythonAvailable(path,nil){return path,nil,nil}
 }
 if runtime.GOOS=="windows"{
  if path,err:=exec.LookPath("py");err==nil{
   for _,prefix:=range [][]string{{"-3.12"},{"-3"}}{
    if candidatePythonAvailable(path,prefix){return path,prefix,nil}
   }
  }
 }
 // If global Python has no suitable packages, use the isolated venv created
 // by install.cmd / install.sh without requiring shell activation.
 if executable,err:=os.Executable();err==nil{
  checkout:=filepath.Clean(filepath.Join(filepath.Dir(executable),".."))
  py:=filepath.Join(checkout,".cusic-tools","venv","Scripts","python.exe")
  if runtime.GOOS!="windows"{py=filepath.Join(checkout,".cusic-tools","venv","bin","python")}
  if info,e:=os.Stat(py);e==nil && !info.IsDir() && candidatePythonAvailable(py,nil){return py,nil,nil}
 }
 return "",nil,errors.New("No compatible Python with NumPy/SciPy found: rerun install.cmd (Windows) or bash install.sh (macOS), or set CUSIC_PYTHON")
}

func Analyze(ctx context.Context,files []File,maxWindows int)(Report,error) {
 var zero Report
 if len(files)==0||len(files)>32{return zero,errors.New("select 1..32 audio files")}
 if maxWindows==0{maxWindows=512}
 if maxWindows<1||maxWindows>2048{return zero,errors.New("maxWindows must be 1..2048")}
 for _,item:=range files {
  if strings.TrimSpace(item.Name)==""||strings.TrimSpace(item.Path)==""{return zero,errors.New("each audio asset needs name and absolute path")}
 }
 python,pre,err:=Python();if err!=nil{return zero,err}
 payload,err:=json.Marshal(Request{Files:files,MaxWindows:maxWindows});if err!=nil{return zero,err}
 deadline,cancel:=context.WithTimeout(ctx,3*time.Minute);defer cancel()
 args:=append(pre,"-c",workerScript)
 cmd:=exec.CommandContext(deadline,python,args...)
 cmd.Stdin=bytes.NewReader(payload)
 var stdout,stderr bytes.Buffer
 cmd.Stdout=&stdout;cmd.Stderr=&stderr
 if err=cmd.Run();err!=nil{
  if deadline.Err()!=nil{return zero,fmt.Errorf("NumPy worker timed out/cancelled: %w",deadline.Err())}
  message:=strings.TrimSpace(stderr.String())
  if len(message)>4000{message=message[:4000]}
  if message==""{message=err.Error()}
  if strings.Contains(message,"No module named"){
   return zero,fmt.Errorf("Python NumPy/SciPy dependency missing: %s; install with python -m pip install -r audio/requirements.txt",message)
  }
  return zero,fmt.Errorf("NumPy worker failed: %s",message)
 }
 if stdout.Len()>8<<20{return zero,errors.New("NumPy worker returned oversized response")}
 decoder:=json.NewDecoder(&stdout);decoder.DisallowUnknownFields()
 var result Report
 if err=decoder.Decode(&result);err!=nil{return zero,fmt.Errorf("invalid Python worker JSON: %w",err)}
 if result.Version!=1||result.Engine!="numpy-scipy"||len(result.Stems)!=len(files){return zero,errors.New("unexpected Python worker response version/results")}
 return result,nil
}
