# cusic agent-first integration

## Sunoステムから新規作成（BPM指定不要）

Cubaseにまだ取り込んでいないSunoのステムだけでも始められます。

```bash
cusic init --name "MySong"
cusic suno import ./stems
cusic tempo analyze
cusic tempo set --bpm 184
cusic setup .
```

テンポ未指定で初期化すると0（未確定）を保存し、偽の120 BPMを採用しません。ドラムステムから推定候補を表示しますが、正しいBPMの判断と確定は人間が行います。音声を実際に184 BPMに合わせたいときは、確認後に `cusic tempo render --out ./aligned-v1 --confirm` を使用。Rubber Band CLIを別途インストールする必要があります。

MCPの `tempo_analyze`、`tempo_confirm`、`tempo_render` も使えます。後二者は明示承認を要求します。曲中の拍ごとの調整（adaptive）は実験機能で、音質と小節位置の検証が必須です。

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
| DAWprojectのVST3設定・インストゥルメント状態・オートメーションの原本保持 | 自動（元DAWproject保存＋変更箇所のみ更新） |
| 汎用Compressorの追加・編集、既存VST3 FXの複製・プリセット状態差替え | MCPで対応範囲を確認、適用前に承認 |
| Kontakt外部チャンネル・パン・ルーティングの診断 | DAWprojectのメタデータから読める範囲で自動 |
| VSQX/SVP/PPSFの原本バックアップ | MCPで自動 |
| 初音ミクの声色探索とテンプレート自動選択 | `vocal_voices` と `score_export` で自動。実際のVSQXの声色情報を再利用 |
| ボーカルトラック作成・歌詞付きノートの追加・部分変更 | `vocal_notes_apply` で同時に処理、既存のフレーズは保持 |
| 既存VSQXの歌詞・音高のみ変更（元のコントローラ残存） | MCPで新ファイル生成 |
| テトSV2を元SVPから複製して旧トラックをミュートし、新トラックへ編曲を適用 | MCP `svp_clone_plan` → `svp_clone_apply`（新SVP生成、承認必須） |
| 日本語の母音無声化・モーラ配分・小節境界の子音先行を解析 | MCP `vocal_phonology_analyze`（読み取り専用） |
| 既存SV2ノートの精密な手動差分編集 | 従来のTS+Zodツール経由、SynthV内で手動実行も引き続き対応 |
| CubaseからDAWprojectを出す・戻す | **手動** |
| Cubaseの開いている画面を操作してEQなどをリアルタイム変更 | **未実装** |
| 既存の調声全体（ノート時間変更、リテイクまで）を完全な意味でAIに編集させる | **未実装** |

## MCPで公開しているツール

`project_inventory`、`project_initialize`、`project_status`、`project_import`、`project_source_status`、`project_source_attach`、`effects_inspect`、`effects_plan`、`effects_apply`、`kontakt_diagnose`、`track_create`、`track_inspect`、`patch_apply`、`history`、`history_revert_head`、`audio_import`、`audio_list`、`mix_analyze_stems`、`mix_plan`、`mix_apply`、`score_export`、`cubase_sync_plan`、`cubase_sync_apply`、`vocal_source_status`、`vocal_backup_source`、`vocal_vsqx_edit_existing`、`vocal_voices`、`vocal_notes_apply`、`vocal_phonology_analyze`、`svp_clone_plan`、`svp_clone_apply`。

## ミックスでの重要な制約

`mix_apply` は元CubaseのDAWprojectのZIP内部にある`project.xml`のミキサー静的値を編集して、**新しいファイル**として書き出す。録音データと既存VSTバイナリはコピーするが、Cubaseの実機での標準Equalizerのマッピング・音響結果は未確認。最初はテスト用コピーで開いて試聴する。

SQLiteに登録したWAVは曲頭からのオーディオステムとして扱う。しかし**既存のDAWprojectを取り込んだ場合は、cusicが元のDAWprojectを丸ごと保存**するため、今後の `cusic export --format dawproject` とMCPの `score_export` はその原本を使う。音符が変更されていなければバイト単位で複製し、変更した場合も単純なノートクリップだけ差し替えて、既存のVST3状態・ルーティング・録音データなどを保持する。安全に差し替えられない複雑な楽曲構造はエラーにする。

すでに古いcusicで取り込んだ曲は `cusic source attach --from original.dawproject`、または `project_source_attach` で元ファイルを紐付ける。自動で保存されたかは `cusic source status` で確認できる。

エフェクト編集は `effects_inspect` → `effects_plan` → `effects_apply` の順。汎用Compressorの挿入、既存VST3 FXとStateの複製、明示的な互換状態ファイルによる更新などをサポートするが、**Kontaktの独自内部ルーティングやVST3の不透明なバイナリ状態を解読して任意ノブを書き換えるものではない**。Cubaseで利用不可のプラグインはDAWproject内に状態を保持してもロードできない。

## ミク V4Xの制作・書き出し（テンプレートを手指定しない）

1. `vocal_voices` で曲フォルダのVSQXから実際の声色（Original/Solid/Sweetなど）を検出する。
2. `vocal_notes_apply` でSideVocalの新規作成と歌詞付きノートの登録を**1回のMCPコールで実行**。既存ノートの変更にはノートIDを指定し、無関係なフレーズを消さない。
3. `score_export` に `format: "vsqx"`、`track: "SideVocal"`、`voice: "Original"`、`out: "side-vocal.vsqx"` を渡す。`template` は省略可能。
4. 本物のOriginal識別情報が見つからないとき、実在する他のボイスバンクを使って書き出し、実際の声色と警告をユーザーへ必ず報告する。架空のOriginal IDを生成しない。

`っ` は無声音、`br1`〜`br5` はブレス指定としてVSQXに出力する。ただしPiaproでの発音結果は実際のボイスバンクと歌唱設定に依存する。**まだ実際のVSQXが1つもない場合のみ、Piaproから声色を含むVSQXを1つ書き出す必要がある。** その後のテンプレート指定は自動。

既存の調声済みVSQXは上書きしない。ノートIDを使ったcusicの差分編集と、原本のmCtrl/nStyleを維持する `vocal_vsqx_edit_existing` は用途が異なる。前者はcusicで管理する新しいボーカルフレーズ、後者はすでに調声されたVSQX向け。

## テトSV2：原本を保持した新トラックへの自動適用

`vocal_phonology_analyze` → `svp_clone_plan` → `svp_clone_apply` が標準ルート。既存SVPのTetoの歌声データベース・調声・リテイクを複製し、新トラックにcusicで変更したノートと歌詞を配置する。元Tetoのすべての情報は残し、新しいSVPの中では元Tetoだけミュートする。**ソースSVPファイルは上書きせず、新しいファイルを作る。** 音符を自動適用するためにユーザーにJavaScriptの手動実行を要求しない。一般的なSVP JSON構造を扱うが、実機での読み込み確認は未実施。

`cusic vocal analyze --track Teto` または `cusic vocal analyze --from source.svp --track Teto` は、日本語の高母音/i,u/が無声化しやすい条件（前後の無声子音・休止）、モーラと音節の差、長い子音によるpreutteranceの小節越えリスクを報告する。例：**「くゆらす」では「く」の後が有声の /j/ なので機械的に母音を消さず、語尾「す」の /u/ は文脈に応じて無声化候補**とする。複数の仮名モーラが1ノートに入っている場合は、新トラックだけを拍内で分割し、開始・終了時刻は元ノートの範囲に閉じる。歌唱で実際に母音が消失するかは音響と歌い方次第で、音源波形の強制アラインメントを行ったと誤認させてはいけない。

CLIからは `cusic svp plan --from original.svp --track Teto` → `cusic svp apply --from original.svp --track Teto --out edited.svp`。旧TypeScript版の `prepare-safe.js` は既存ノートの精密な手動調声用として引き続き利用可能。

## 歌声調声保護の重要な制約

バックアップは完全コピーなので原本への復元ができる。一方、歌詞や音高を変更しても歌唱結果が完全に同じになる保証はない。既存のピッチやノートスタイルのXML属性を残す方式であり、任意の音符の追加、削除、時間移動、分割をして調声・表情を完全に引き継ぐ仕組みではない。

SV2の安全編集は`npm install && npm run build`後、`node dist/host/prepare-safe.js edits.json --out new-script.js`でビルド成果物を準備し、Synthesizer V Studio 2 Pro上でJSを手動実行する。調声済みの範囲に従来の完全置換スクリプトを使わないこと。

## 今後の課題

本当の完全自然文オペレーションには、Cubase側のエクスポート/インポートを自動操作する安全なブリッジ、プラグイン固有パラメータの取り扱い、既存SVPの非破壊パッチのさらなる拡張、実機互換性テストが必要。MIDI Remote APIだけでファイルの自動書き出しが保証されるわけではない。

## Python/NumPy FFT解析

本体・MCP・SQLiteはGo。テトだけTypeScript＋Zod。音響解析はPython・NumPy・SciPyで行う。1回だけ py -m pip install -r audio/requirements.txt を実行。AIはMCPの mix_analyze_spectral を呼び出し、登録WAVのFFT、8帯域RMS、周波数重心、スペクトル類似度、ステレオ相関を得る。長尺音源はサンプル窓からの推定値で、LUFSや心理音響的マスキングそのものではない。Go製の簡易mix_analyze_stemsも引き続き利用可能。
