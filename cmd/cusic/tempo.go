package main

import (
    "context"
    "encoding/json"
    "errors"
    "flag"
    "fmt"
    "math"
    "os"
    "os/exec"
    "path/filepath"
    "strings"

    "github.com/ichiken26/cusic/internal/store"
    "github.com/ichiken26/cusic/internal/tempo"
)

// Select a registered drum stem in preference to other instruments. Don't
// silently infer tempo from a random guitar/voice if the user has many stems.
func tempoSource(s *store.Store, selector string) (store.Asset, string, error) {
    assets,err:=s.Assets()
    if err!=nil{return store.Asset{},"",err}
    if len(assets)==0{return store.Asset{},"",errors.New("no WAV assets: run cusic suno import ./stems")}
    var selected *store.Asset
    if selector!="" {
        for i:=range assets {
            if strings.EqualFold(assets[i].Name,selector) || strings.EqualFold(assets[i].ID,selector) {
                selected=&assets[i]
                break
            }
        }
        if selected==nil {return store.Asset{},"",fmt.Errorf("audio asset %q not found (use cusic audio list)",selector)}
    } else {
        for i:=range assets {
            n:=strings.ToLower(assets[i].Name)
            if strings.Contains(n,"drums") || strings.Contains(n,"drum") || strings.Contains(n,"percussion"){
                selected=&assets[i]
                break
            }
        }
        if selected==nil && len(assets)==1 { selected=&assets[0] }
        if selected==nil {return store.Asset{},"",errors.New("no obvious drum stem: use --source NAME from cusic audio list")}
    }
    path,err:=s.AssetAbsolute(*selected)
    return *selected,path,err
}
func analyzeTempo(s *store.Store,source string)(tempo.Report,store.Asset,error){
    asset,path,err:=tempoSource(s,source)
    if err!=nil{return tempo.Report{},asset,err}
    report,err:=tempo.Analyze(context.Background(),path)
    return report,asset,err
}
func tempoCommand(s *store.Store,args []string)error {
    if len(args)==0 {return errors.New("usage: cusic tempo analyze [--source NAME] | set --bpm N | render --out DIR --confirm [--mode uniform|adaptive]")}
    switch args[0] {
    case "analyze":
        f:=flag.NewFlagSet("tempo analyze",flag.ContinueOnError)
        source:=f.String("source","","registered drum stem name (optional)")
        if err:=f.Parse(args[1:]);err!=nil{return err}
        if f.NArg()!=0{return errors.New("unexpected arguments to tempo analyze")}
        report,asset,err:=analyzeTempo(s,*source)
        if err!=nil{return err}
        return printJSON(map[string]any{"source":asset.Name,"report":report,"hint":"Estimated BPM is provisional: inspect candidates and listen; cusic tempo set --bpm N confirms it. No WAV has been modified."})
    case "set":
        f:=flag.NewFlagSet("tempo set",flag.ContinueOnError)
        bpm:=f.Int("bpm",0,"confirmed project tempo (20..400)")
        if err:=f.Parse(args[1:]);err!=nil{return err}
        if f.NArg()!=0{return errors.New("unexpected arguments to tempo set")}
        revision,err:=s.SetInitialTempo(*bpm)
        if err!=nil{return err}
        fmt.Printf("Confirmed %d BPM, revision %d. Original WAVs unchanged.\n",*bpm,revision)
        return nil
    case "render":
        f:=flag.NewFlagSet("tempo render",flag.ContinueOnError)
        out:=f.String("out","","new output folder for corrected WAV files")
        mode:=f.String("mode","uniform","uniform or adaptive (experimental beat tracking)")
        source:=f.String("source","","registered reference stem")
        sourceBPM:=f.Float64("source-bpm",0,"override uncertain inferred source BPM (uniform only)")
        confirm:=f.Bool("confirm",false,"explicitly authorize creation of new processed WAVs")
        if err:=f.Parse(args[1:]);err!=nil{return err}
        if f.NArg()!=0{return errors.New("unexpected arguments to tempo render")}
        if !*confirm{return errors.New("render requires --confirm after reviewing cusic tempo analyze")}
        if *out==""{return errors.New("--out NEW_DIRECTORY required")}
        if *mode!="uniform" && *mode!="adaptive"{return errors.New("--mode must be uniform or adaptive")}
        if *sourceBPM!=0 && (*sourceBPM<20||*sourceBPM>400 || *mode!="uniform") {return errors.New("--source-bpm must be 20..400 and is only supported with uniform mode")}
        settings,err:=s.Settings();if err!=nil{return err}
        if settings.Tempo<20{return errors.New("project BPM not confirmed: run cusic tempo analyze then cusic tempo set --bpm N")}
        report,asset,err:=analyzeTempo(s,*source);if err!=nil{return err}
        from:=report.EstimatedBPM
        if *sourceBPM!=0 {from=*sourceBPM}
        if report.Confidence<0.20 && *sourceBPM==0{return errors.New("tempo evidence too weak; confirm a source BPM and pass --source-bpm")}
        if *mode=="adaptive" && (report.Confidence<0.45 || report.BeatCoverage<0.35 || len(report.BeatTimesSeconds)<16){
            return errors.New("adaptive beat alignment is too uncertain; use uniform mode or improve the drum stem")
        }
        return renderTempoStems(s,*out,*mode,from,float64(settings.Tempo),asset.Name,report)
    }
    return errors.New("usage: cusic tempo analyze|set|render")
}

func renderTempoStems(s *store.Store,out,mode string,from,target float64,sourceName string,report tempo.Report)error{
    binary,err:=exec.LookPath("rubberband")
    if err!=nil{return errors.New("rubberband executable missing; install rubberband-cli (pitch-preserving timestretch)")}
    assets,err:=s.Assets();if err!=nil{return err}
    if len(assets)==0||len(assets)>32{return errors.New("render supports 1..32 WAV stems")}
    reference:=assets[0].Stats.Duration
    for _,a:=range assets {
        if math.Abs(a.Stats.Duration-reference)>0.05 {return fmt.Errorf("stem %q does not match song length: export synchronized WAVs first",a.Name)}
    }
    out,err=filepath.Abs(out);if err!=nil{return err}
    if _,err=os.Stat(out);err==nil{return errors.New("output directory exists; choose a fresh destination")}else if !errors.Is(err,os.ErrNotExist){return err}
    parent:=filepath.Dir(out)
    if err=os.MkdirAll(parent,0755);err!=nil{return err}
    tempDir,err:=os.MkdirTemp(parent,".cusic-render-*")
    if err!=nil{return err}
    defer os.RemoveAll(tempDir)
    ratio:=from/target
    if ratio<0.25 || ratio>4 {return errors.New("suspicious tempo ratio: verify half/double-time interpretation")}
    summary:=map[string]any{"sourceStem":sourceName,"sourceBpm":from,"targetBpm":target,
        "mode":mode,"inputDurationSeconds":reference,"analysis":report,
        "warning":"Compare new WAVs with originals: time-stretching is not lossless, and beat detections may be inaccurate."}
    b,err:=json.MarshalIndent(summary,"","  ");if err!=nil{return err}
    if err=os.WriteFile(filepath.Join(tempDir,"tempo-report.json"),append(b,'\n'),0644);err!=nil{return err}
    for index,a:=range assets {
        input,e:=s.AssetAbsolute(a);if e!=nil{return e}
        output:=filepath.Join(tempDir,fmt.Sprintf("%02d-aligned.wav",index+1))
        args:=[]string{"--fine","--time",fmt.Sprintf("%.12f",ratio)}
        if mode=="adaptive"{
            mapping,e:=makeTimeMap(report.BeatTimesSeconds,a.Stats.Duration,a.Stats.SampleRate,target,ratio)
            if e!=nil{return e}
            name:=filepath.Join(tempDir,fmt.Sprintf("%02d-timemap.txt",index+1))
            if e=os.WriteFile(name,[]byte(mapping),0644);e!=nil{return e}
            args=append(args,"--timemap",name)
        }
        args=append(args,input,output)
        cmd:=exec.Command(binary,args...)
        if outputLog,e:=cmd.CombinedOutput();e!=nil{
            if len(outputLog)>1500 {outputLog=outputLog[len(outputLog)-1500:]}
            return fmt.Errorf("rubberband failed for %s: %w (%s)",a.Name,e,string(outputLog))
        }
        fmt.Printf("Rendered %s -> %s\n",a.Name,filepath.Base(output))
    }
    if err=os.Rename(tempDir,out);err!=nil{return err}
    fmt.Printf("Rendered %d synchronized stems in %s. Original assets unchanged.\n",len(assets),out)
    return nil
}

// Each input/source beat maps to an evenly spaced output beat. The map is
// shared in seconds among all stems and independently converted to frames for
// each sample rate; this keeps the original multitrack alignment intact.
func makeTimeMap(beats []float64,duration float64,sampleRate int,targetBpm float64,ratio float64)(string,error){
    if len(beats)<16||duration<=0||sampleRate<8000{return "",errors.New("insufficient adaptive time map")}
    step:=60/targetBpm
    offset:=beats[0]
    var b strings.Builder
    b.WriteString("0 0\n")
    lastSource,lastTarget:=int64(0),int64(0)
    for i,t:=range beats{
        if t<=0||t>=duration{continue}
        destination:=offset+float64(i)*step
        sourceFrame:=int64(math.Round(t*float64(sampleRate)))
        targetFrame:=int64(math.Round(destination*float64(sampleRate)))
        if sourceFrame<=lastSource||targetFrame<=lastTarget{return "",errors.New("non-monotonic beat anchors; use uniform mode")}
        // Extreme local warping usually means a misdetected attack.
        localRatio:=float64(targetFrame-lastTarget)/float64(sourceFrame-lastSource)
        if localRatio<0.75 || localRatio>1.25{return "",errors.New("unreliable beat anchor: required stretch exceeds 25%; use uniform mode")}
        fmt.Fprintf(&b,"%d %d\n",sourceFrame,targetFrame)
        lastSource,lastTarget=sourceFrame,targetFrame
    }
    endSource:=int64(math.Round(duration*float64(sampleRate)))
    endTarget:=int64(math.Round((float64(lastTarget)/float64(sampleRate)+(duration-float64(lastSource)/float64(sampleRate))*ratio)*float64(sampleRate)))
    if endSource<=lastSource||endTarget<=lastTarget{return "",errors.New("invalid final stretch range")}
    fmt.Fprintf(&b,"%d %d\n",endSource,endTarget)
    return b.String(),nil
}
