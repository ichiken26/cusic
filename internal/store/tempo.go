package store

import (
    "errors"
    "fmt"
    "encoding/json"
)

// SetInitialTempo records the explicitly confirmed BPM. An audio-first project
// starts at Tempo=0, never a fictitious 120 BPM. Existing scores are protected.
func (s *Store) SetInitialTempo(bpm int) (int64, error) {
    if bpm<20 || bpm>400 {return 0, errors.New("tempo must be 20..400 BPM")}
    tx,err:=s.DB.Begin()
    if err!=nil{return 0,err}
    defer tx.Rollback()
    var rawSettings string
    if err=tx.QueryRow("SELECT value FROM settings WHERE key='project'").Scan(&rawSettings);err!=nil{return 0,err}
    var old Settings
    if err=json.Unmarshal([]byte(rawSettings),&old);err!=nil{return 0,err}
    if old.Tempo!=0 {
        if old.Tempo==bpm {return 0,fmt.Errorf("project tempo is already %d BPM",bpm)}
        return 0,fmt.Errorf("project already uses %d BPM; refusing to retime existing notes",old.Tempo)
    }
    updated:=old
    updated.Tempo=bpm
    next,err:=json.Marshal(updated)
    if err!=nil{return 0,err}
    if _,err=tx.Exec("UPDATE settings SET value=? WHERE key='project'",string(next));err!=nil{return 0,err}
    rev,err:=record(tx,"tempo.confirm",fmt.Sprintf("Confirm project tempo %d BPM",bpm),
        []Change{newChange("settings","project",old,updated)})
    if err!=nil{return 0,err}
    return rev,tx.Commit()
}
