# cusic

**Cubase + music** — Claude Code / Codex と一緒に楽曲を作るための、Go製ヘッドレスCLI。
楽曲の音符と編集履歴は楽曲ごとのSQLite（`.cusic/project.db`）に保存する。cusic内部でGitは使わない。GUIも持たない。

> **開発中 / experimental**：ソースコードの変換・MIDI往復テストはCIで検証するが、Cubase / Piapro Studio V4X / Synthesizer V 2 Proの**実機による受け入れテストは未実施**。実制作前にテストプロジェクトとバックアップで確認してね。各ファイル形式の全機能を保持する変換ではない。

## Claude Code / Codex：曲フォルダで自然文だけで操作（NEW）

**初回に一度だけ `cusic setup`。以降はClaude Code / Codexで曲フォルダを開いて自由文で指示できる。**

1. Cubaseで既存の `.cpr` を開き、DAWprojectを**1本**書き出して曲フォルダに置く（初回と変更後の再同期時に必要）。
2. Windows / macOS / Linux版のcusicバイナリを使い、曲フォルダで `cusic setup` を実行する。
3. `.mcp.json`（Claude）と `.codex/config.toml`（Codex）、エージェント向け説明が自動生成される。1本だけDAWprojectがある場合はSQLite初期化とMIDI/WAV取り込みも実行される。
4. Claude Code / Codexを**曲フォルダ**で開き、ローカルMCPの利用を許可して、例えば「ギター2本・ベース・ドラム・ミク・テトの音の重なりを解析して、ミクとテトが埋もれないミックス案を作って。元のデータは絶対に変更しないで」と指示する。

```sh
cusic setup "D:\Music\MySong"
```

cusic MCPには `project_inventory`、`project_status`、`track_inspect`、`project_import`、`audio_import`、`mix_analyze_stems`、`mix_plan`、`mix_apply`、`vocal_backup_source`、`vocal_vsqx_edit_existing`、`patch_apply`、`cubase_sync_plan` などを公開。ユーザーがCLIのコマンド体系を覚える必要はない。

### Cubase自動化の現状

**完全無人のCubase操作ではない。** `.cpr`を直接解析したり、開いているCubaseの録音トラックを自動書き出ししたり、ミキサーをライブ制御したりはできない。Cubaseから**DAWprojectを1本書き出す**操作、変更済みDAWprojectを取り込む操作は手動。単なる個別のWAV＋MIDI一括書き出しよりも、DAWprojectが構成情報を残せるため基本経路としている。

**既存曲のミックスでは、Cubaseから書き出した元のDAWprojectを `mix_apply` のソースとして使用。** cusicのSQLiteから再生成したDAWprojectでは、録音の任意のクリップ配置やワープを完全復元できない。`mix_analyze_stems` はWAVのピーク/RMS/粗い帯域エネルギーを測定、`mix_plan` でEQ・パン・フェーダーの案を事前確認し、承認後の `mix_apply` が**別名のDAWproject**を生成する。オリジナルDAWproject内の録音・プラグインバイナリはコピーするが、Cubase実機での汎用EQの読み込み互換性は未検証。

### ミクV4X・テトSV2の既存調声を守る

- `vocal_backup_source` で元の `.vsqx` / `.svp` / `.ppsf` を**SHA-256付きでバイト単位バックアップ**。これは元のピッチ・表情・リテイクを丸ごと復元できる。
- ミク：`vocal_vsqx_edit_existing` で**既存音符の音高・歌詞だけ**新しいVSQXに変更。元の`mCtrl`や`nStyle`、タイミング、音源識別子を保持。
- テト：`synthv/src/safe.ts` からビルドしたES5.1のJSで、**既存ノートを削除せず**音高・歌詞だけ変更。TypeScript＋Zod検証はホストで実行、SV2上では生成JSを実行して**別名SVPとして保存**。実機での挙動は未確認。
- 既存の音符の長さやタイミングを変えた場合にカーブやリテイクを安全移設する機能は未実装。歌唱調声全体をcusicのSQLiteに完全同期しているわけではない。旧`prepare.js`の完全置換は**新規フレーズ専用**。

詳しい利用手順・制約は [docs/agent-first.md](docs/agent-first.md) を参照。

---
### NumPy/SciPyによる波形・FFT解析（追加）

波形の高度な計算は **Python + NumPy/SciPy** のオフラインワーカーで行う。GoはMCP・SQLite・ファイル管理、TypeScriptはテトSV2のスクリプト担当。Goだけで行う簡易解析とは別に、NumPyのrFFTを使って音響解析する。

**Windowsは `install.cmd` でPython 3.12・NumPy・SciPyをまとめて自動構築する。** 既存のPythonやpip、Docker Desktopは不要。Python本体と依存パッケージは `.cusic-tools/` 内の仮想環境へ隔離され、Windows全体のPython環境は変更しない。

cusicのGoバイナリは実行ファイルの位置から `.cusic-tools/venv/Scripts/python.exe` を自動検出して利用する。標準以外のPythonを使いたい場合のみ `CUSIC_PYTHON` で上書き可能。macOS/LinuxでのPython依存は現状手動導入。

登録済みWAVステムなら次のコマンドで測定可能。

~~~powershell
cusic suno import .\stems
cusic analyze --max-windows 512
~~~

通常はClaude CodeやCodexに「6トラックの周波数をNumPyで解析して、ギター2本とミク・テトが干渉しそうな帯域を探して」と頼む。MCPツール mix_analyze_spectral が自動で呼び出せる。

返す数値：8帯域のRMS dBFS、周波数重心・95%ロールオフ、代表的なピーク周波数、ステレオ相関、パート間の帯域スペクトル類似度、サンプル窓のピーク・RMS。ファイルは読み込み専用で、録音を加工・上書きしない。

**注意：**長い音源は均等に抽出したFFT窓による近似。LUFS・真のピーク・聴覚上のマスキングを直接測る機能ではない。EQの提案はCubaseで試聴して評価する。

---

## できること

| 入出力 | コマンド | 対応範囲 |
|---|---|---|
| MIDI | `cusic import song.mid` / `cusic export --format midi` | SMF 0/1、固定テンポ・拍子の音符 |
| TuxGuitar | `cusic import song.tg` | **2.x形式（ZIP + content.xml）**の音符・チューニング・簡易タイ |
| NumPy FFT | cusic analyze / MCP mix_analyze_spectral | rFFT、8帯域RMS、ステレオ相関、複数ステムの帯域類似度 |
| Suno WAV | `cusic suno import ./stems` | ステムWAVの長さ、ピーク、RMS、SHA-256、SQLite登録 |
| Suno分離 | `cusic suno separate --out ./stems mix.wav` | 別途インストールした **Demucs** を実行 |
| Suno→MIDI | `cusic suno transcribe --out ./midi stem.wav` | 別途インストールした **Basic Pitch** を実行（推定精度は音源依存） |
| Cubase | `cusic export --format dawproject --out song.dawproject` | 音符トラックとタイムライン先頭に揃えたWAVステムをZIPに格納 |
| Cubaseから取り込み | `cusic import edit.dawproject` | DAWproject内の基本的な音符クリップ・WAVを新規トラックとして読み込む |
| Cubase編集の同期 | `cusic sync plan/pull --from edit.dawproject` | 同名の**楽器トラックの音符のみ**、手動・差分確認付き同期 |
| 初音ミク V4X | `cusic export --format vsqx --template empty.vsqx --track Miku --out miku.vsqx` | Piapro Studioで書き出したVOCALOID4テンプレートを利用 |
| 重音テト SV2 | `cusic export --format synthv --track Teto --bars 1:8 --out teto.json` | TypeScript + Zod検証→ビルド済みES5.1 JSでSynthVへ反映 |
| 履歴 | `cusic status/log/diff/revert` | SQLiteトランザクション単位の更新履歴。revertはHEADのみ |

**Cubaseの `.cpr` ファイルを直接変更したり、画面をクリックして操作したり、編集中のCubaseとリアルタイム同期したりはしない。**
CubaseではDAWprojectをインポート/エクスポートして受け渡す。

## インストール

**Windowsの利用者はGoもPythonも事前インストール不要。** 初回にcusicがGo 1.26.10・uv 0.13.7・Python 3.12・NumPy/SciPy・Rubber Bandをインターネットから取得して専用フォルダに構築する。取得するGo/uv/Rubber BandのアーカイブはSHA-256を照合する。SynthV用JSのソースを自分でビルドする場合のみNode.js 22+が別途必要。

**Windows（推奨・最初の1回だけ）**

~~~cmd
git clone https://github.com/ichiken26/cusic.git
cd cusic
.\install.cmd
~~~

`install.cmd` が専用のGoを自動インストールして `bin\cusic.exe` をビルドし、**現在のWindowsユーザーのPATH**に絶対パスで `bin` を永続登録する。管理者権限も `$env:Path += ...` も不要。再実行してもPATHに同じ項目を追加しない。Go/Python/uvは全体のPATHに追加しない。

また、BPM補正に使う **Rubber Band CLI** も自動セットアップする。既にPATH上にあればそのまま利用し、なければ[公式のWindows版](https://breakfastquay.com/rubberband/)（v4.0.0）をダウンロードして公開済みSHA-256を検証し、`bin/rubberband.exe` と必要なDLL・ライセンスファイルを展開。**初回だけネット接続が必要**。Scoopの導入や手作業のPATH追加は不要。ダウンロード・ハッシュ検証に失敗した場合はインストールを失敗として通知し、未検証の実行ファイルを利用しない。

`cusic tempo analyze` やスペクトル解析で使うPython + NumPy/SciPyも、uvを利用して `.cusic-tools/venv` に**自動インストール**する。既存のPython環境を変更しない。初回はGo・Pythonのランタイム取得で相応のダウンロード容量が必要。

**インストール後にターミナルを再起動**（VS Codeの統合ターミナルならVS Code自体を再起動）してから、どの曲フォルダでも `cusic help` を実行できる。現在開いている別プロセスのPATHまでは変更できない。リポジトリのフォルダを移動した場合は、移動後に `install.cmd` を再実行してね。

Windowsでセットアップが完了したら、手動の `pip install`、`go install`、`rubberband` インストールは不要。Pythonのパッケージに更新があった場合も `install.cmd` を再実行すれば専用の仮想環境に同期する。

**macOS / Linux**

~~~bash
git clone https://github.com/ichiken26/cusic.git
cd cusic
go build -o ./bin/cusic ./cmd/cusic
./bin/cusic help
~~~

Windows/macOS/LinuxのバイナリとSynthV用JSはGitHub Actionsの成功した実行の **Artifacts** からも取得できる。以降の例では `cusic` がPATHにある前提。

## 楽曲を開始

~~~bash
mkdir my-song && cd my-song
cusic init --name "My Song" --tempo 168 --meter 4/4
cusic track add --name Bass --kind instrument
cusic track add --name Teto --kind vocal --singer teto_sv2
cusic track add --name Miku --kind vocal --singer miku_v4x
cusic status
~~~

1拍=480 ticks、4/4なら1小節=1920 ticks。各コマンドは現在のディレクトリから親を辿って `.cusic/project.db` を探す。別の場所にあるなら `CUSIC_PROJECT=/abs/path` を設定する。

### TuxGuitar / MIDIを読み込む

~~~bash
cusic import /path/to/song.tg
cusic import /path/to/song.mid
cusic track list
cusic inspect --track Bass --bars 17:24
~~~

`.tg` の直接読み込みは**TuxGuitar 2.xのZIP/XML形式だけ**。1.xのバイナリ `.tg` はTuxGuitarからMIDIを書き出して読み込む。ギターの運指、アーティキュレーション、ベンド、装飾などは現在の楽曲モデルに保持しない。テンポ/拍子の途中変更がある曲は、安全のため拒否する。

### Claude Code / Codexに部分編集を任せる

~~~bash
cusic status
cusic inspect --track Bass --bars 17:24
# AIが現在のHEADをbaseRevisionに指定したpatch.jsonを生成
cusic patch apply ./patch.json
cusic diff 12
cusic log
cusic validate
cusic revert 12 # 12がHEADのときだけ
~~~

`examples/patch.json` に例がある。**指定トラックの指定小節に含まれる音符を完全置換**するため、残す音符もパッチに必ず含める。範囲をまたぐノートや古いHEADへのパッチは拒否する。修正後はCubaseに新しいファイルをエクスポートして聴き比べる。

## BPM指定なしでSunoステムから始める

SunoのWAVがあるなら、新規プロジェクト作成時にBPMを知っている必要はありません。

~~~bash
cusic init --name "MySong"
cusic suno import ./stems
~~~

- `--tempo` を省略すると **未確定（tempo: 0）** となり、従来の120 BPMを勝手に設定しません。既存の `cusic init --tempo 184` は引き続き使えます。
- `suno import` はWAVをコピー・登録し、ドラムステムがあればNumPy/SciPyでテンポ候補と確信度を表示します。元のWAVは加工しません。
- Python未導入や無音・短いWAVで解析できなくても、**音声の取り込み自体は完了**します。
- 検出結果は **推定**。特に92/184 BPMのような半分・倍テンポ、裏拍や曲中のテンポ揺れには注意して、曲を聴きながら決めてください。

~~~bash
python -m pip install -r audio/requirements.txt
cusic tempo analyze                     # いつでも再解析可能
cusic tempo analyze --source Drums      # ステム名を明示して解析
cusic tempo set --bpm 184               # 人間が確認したBPMをSQLiteへ記録
cusic status
cusic export --format dawproject --out MySong.dawproject
~~~

**BPM確定前のMIDI/DAWproject/歌声データの書き出しはエラーになります。** 既存のテンポがあるプロジェクトのBPMを暗黙に上書きすることもありません。

### 全ステムを同じ時間補正で新しいWAVへ（任意）

Rubber Bandのコマンドライン版 `rubberband` が別途必要です。全ステムが同じ曲頭・長さで揃ったSuno WAVにのみ使用してください。

~~~bash
cusic tempo analyze
cusic tempo set --bpm 184
cusic tempo render --out ./aligned-v1 --mode uniform --confirm
# 変動テンポを拍単位で補正する実験機能（推定拍が明確な場合だけ）
cusic tempo render --out ./aligned-beats-v1 --mode adaptive --confirm
# テンポ推定の解釈が半分/倍と判明した場合、uniformのみ手動の元BPMを指定可能
cusic tempo render --out ./aligned-v2 --source-bpm 183.5 --confirm
~~~

`uniform` は推定元BPMと確定先BPMの比率で一定の長さ補正、`adaptive` は推定した拍の位置からRubber Band用の可変タイムマップを生成します。後者は**実験的**で、拍検出に失敗すると演奏のタイミングを悪化させる恐れがあります。タイムストレッチは無劣化ではありません。

- 処理前に必ず `tempo analyze` の候補をレビューして、**新しい出力ディレクトリ名**を指定してください。
- 元WAVもcusic内部の元アセットも上書きしません。失敗時は途中成果物を削除します。
- 音源が同期していない／尺が違う場合は処理を拒否。全ステムに同じ秒単位のタイムマップを使います。
- 補正後の音がよければ `cusic suno import ./aligned-v1` で**別アセットとして取り込む**ことができます。既存の未補正WAVも残ります。
- 間違った推定を自動承認することはありません。実際のCubase再生で各パートの同期とクリックを確認してください。

Claude/Codex MCPにも `tempo_analyze`, `tempo_confirm`, `tempo_render` を公開し、確認が必要な処理は承認済みフラグを要求します。初期化 `project_initialize` でもBPMを省略できます。

## Sunoの音源

Suno Studioから書き出した複数のWAVステムを、音符とは別のアセットとして保存する。

~~~bash
cusic suno import /path/to/suno-stems
cusic audio list
cusic export --format dawproject --out cubase-arrangement.dawproject
~~~

Demucsのように複数階層で書き出されるディレクトリも再帰的に読み込める。SunoステムはSHA-256を付けて `.cusic/audio/` にコピーし、SQLiteにはファイル位置とWAV解析メタデータのみ記録する。対応はPCM 16/24/32-bit / Float 32/64-bit RIFF WAV（RF64などは未対応）。

WAVしかないとき、**任意の追加インストール**で分離や推定MIDIが使える。

~~~bash
# 先に別途 pip 等で demucs と basic-pitch コマンドをインストール
cusic suno separate --out ./separated-v1 /path/to/mix.wav
cusic suno transcribe --out ./transcription-v1 /path/to/bass.wav
# 推定結果をCubaseで確認してから...
cusic import ./transcription-v1/<generated-file>.mid
~~~

Demucsは分離処理、Basic Pitchは**単一楽器ステムからの音高推定**用。どちらもcusic内部にモデルを配布するものではなく、PATH上の外部コマンドを起動する。モデルのダウンロード・利用規約・実行環境は各ツールに従う。自動で元曲の正確な原譜になるものではない。

## Cubase ⇄ cusic（DAWproject）

~~~bash
# cusic → Cubase
cusic export --format dawproject --out cubase-v1.dawproject
# CubaseでインポートしてMIDIトラックを編集し、別名でDAWprojectをエクスポート

# Cubase → cusic（先に差分を確認）
cusic sync plan --from cubase-edited.dawproject
# planで確認したbaseRevisionを指定
cusic sync pull --from cubase-edited.dawproject --base-revision 12
cusic log
~~~

同期は**Cubaseでエクスポートしたファイルを明示的に読み込む方式**。同名の楽器トラックを比較し、変更されたトラックの音符をSQLiteリビジョンに記録する。競合したHEADは拒否する。歌詞・歌唱表現の喪失防止のため、`kind=vocal` は同期対象外。Cubase固有のVST設定、CC、オートメーション、テンポマップ、拍の複雑な構成も維持・同期しない。

実機でのDAWproject互換性は未確認なので、**既存Cubaseプロジェクトは上書きせずテスト用のコピーで使ってね。** エクスポートは既存ファイル名を上書きしない。

## ミク V4X（VSQX）

初音ミクV4Xの歌声ライブラリ情報を安全に維持するため、**手元のPiapro Studioから書き出したVSQXをテンプレート**に使う。ミクを設定した**1トラック・1パート・固定テンポ拍子**の短いテンプレートを用意してね。

~~~bash
cusic export --format vsqx --track Miku --template /path/to/miku-template.vsqx --out ./miku-v1.vsqx
~~~

テンプレートの歌手・声ライブラリ情報は保持し、音符・歌詞・基本発音を置き換える。歌詞は**1ノートにつき1つのひらがな/カタカナのモーラ**を推奨。漢字、複雑な促音、特殊な音節など対応外の発音はエラーになる。**Piapro Studio側で音素・ブレス・歌唱パラメータの最終調整が必要**。Piapro実機でのインポート確認は未実施。

## テト SV2（Synthesizer V Studio 2 Pro）

~~~bash
cusic export --format synthv --track Teto --bars 1:8 --out /absolute/path/teto.json
cd /path/to/cusic/synthv
npm install
npm run build
node dist/host/prepare.js /absolute/path/teto.json --out /absolute/path/cusic-teto.js
~~~

JSの作成前にTypeScript+Zodで入力を検証し、SynthVスクリプトランタイムで動かす成果物は**ES5.1にコンパイルしたJS**。SynthVのスクリプトフォルダに配置し、エディタから選択した**同名トラックのmain group**に実行する。範囲外や境界を跨ぐノートは拒否する。対象範囲内の既存の調声は失われることがあるので、SynthVプロジェクトのバックアップを推奨する。

## 開発・制約

~~~bash
go test -mod=mod ./...
go vet -mod=mod ./...
cd synthv
npm install
npm run build
npm test
~~~

楽曲履歴はGitではなくSQLiteの`revisions`に保存し、`revert`はHEADだけ。WAL利用中のDBは単なるファイルコピーでなくSQLiteのバックアップ手段で保存する。

主な未対応範囲：`.tg` 1.x、MusicXML、歌声合成の調声完全同期、リアルタイムCubase制御、DAWproject内の任意プラグイン・オートメーション、可変テンポ・変拍子、Suno専用APIとの直接認証連携。テキストでのAI編曲と演奏データ生成が中心のツール。

Claude CodeとCodexは [AGENTS.md](AGENTS.md) を参照。Claude Codeはさらに [CLAUDE.md](CLAUDE.md) を読む。
