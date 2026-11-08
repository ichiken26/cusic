# cusic agent-first integration

## 最初の1回だけ

1. Cubaseで元の曲を開き、**DAWprojectを1本**エクスポートして曲フォルダに置く。
2. `cusic setup "D:\Music\MySong"` を実行。
3. Claude CodeかCodexをそのフォルダで開く。ローカルMCPサーバーの利用許可があれば承認する。
4. あとは自由文で頼む。例えば「ギター2本とベース、ドラム、ミクV4X、テトSV2の帯域を分析して、録音と既存の調声を守りつつミックス改善案を作成して。良さそうなら別名のDAWprojectに反映して」。

生成ファイル：`.mcp.json`（Claude Code）、`.codex/config.toml`（Codex）、`CUSIC_WORKFLOW.md`、`AGENTS.md` / `CLAUDE.md`（未作成なら作成）。
唯一のDAWprojectが見つかればSQLiteも初期化し、音符と埋め込みWAVを取り込む。ない場合は設定だけ作成するため、AIはMCPから状態確認して初期化を案内する。

## 自動化できる部分

| 処理 | 対応 |
|---|---|
| 曲フォルダ内ファイルの発見 | MCPで自動 |
| DAWproject / MIDI / TuxGuitar2の基本ノート取り込み | MCPで自動 |
| WAV解析（Peak、RMS、大まかな帯域） | MCPで自動 |
| AIの編曲案→範囲指定の楽器ノート編集 | MCPで自動 |
| ミックス案の検証とEQ/フェーダー/パンのDAWproject変換 | MCPで自動、適用前に承認 |
| VSQX/SVP/PPSFの原本バックアップ | MCPで自動 |
| 既存VSQXの歌詞・音高のみ変更（元のコントローラ残存） | MCPで新ファイル生成 |
| 既存SV2の歌詞・音高のみ変更（ノート再生成なし） | TS+Zodツール経由、SynthV内で手動実行 |
| CubaseからDAWprojectを出す・戻す | **手動** |
| Cubaseの開いている画面を操作してEQなどをリアルタイム変更 | **未実装** |
| 既存の調声全体（ノート時間変更、リテイクまで）を完全な意味でAIに編集させる | **未実装** |

## MCPで公開しているツール

`project_inventory`、`project_initialize`、`project_status`、`project_import`、`track_create`、`track_inspect`、`patch_apply`、`history`、`history_revert_head`、`audio_import`、`audio_list`、`mix_analyze_stems`、`mix_plan`、`mix_apply`、`score_export`、`cubase_sync_plan`、`cubase_sync_apply`、`vocal_source_status`、`vocal_backup_source`、`vocal_vsqx_edit_existing`。

## ミックスでの重要な制約

`mix_apply` は元CubaseのDAWprojectのZIP内部にある`project.xml`のミキサー静的値を編集して、**新しいファイル**として書き出す。録音データと既存VSTバイナリはコピーするが、Cubaseの実機での標準Equalizerのマッピング・音響結果は未確認。最初はテスト用コピーで開いて試聴する。

SQLiteに登録したWAVは曲頭からのオーディオステムとして扱う。Cubaseの録音テイクやオーディオイベントの元位置、スリップ、ワープを完全に再現するものではない。**既存曲のミックスは、`cusic export --format dawproject`で作り直したファイルでなく、必ずCubaseからエクスポートした元のDAWprojectを編集する。**

## 歌声調声保護の重要な制約

バックアップは完全コピーなので原本への復元ができる。一方、歌詞や音高を変更しても歌唱結果が完全に同じになる保証はない。既存のピッチやノートスタイルのXML属性を残す方式であり、任意の音符の追加、削除、時間移動、分割をして調声・表情を完全に引き継ぐ仕組みではない。

SV2の安全編集は`npm install && npm run build`後、`node dist/host/prepare-safe.js edits.json --out new-script.js`でビルド成果物を準備し、Synthesizer V Studio 2 Pro上でJSを手動実行する。調声済みの範囲に従来の完全置換スクリプトを使わないこと。

## 今後の課題

本当の完全自然文オペレーションには、Cubase側のエクスポート/インポートを自動操作する安全なブリッジ、プラグイン固有パラメータの取り扱い、既存SVPの非破壊パッチのさらなる拡張、実機互換性テストが必要。MIDI Remote APIだけでファイルの自動書き出しが保証されるわけではない。
