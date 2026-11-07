# cusic

**Cubase + music** — Claude Code / Codex と一緒に楽曲を作るための、Go製ヘッドレスCLI。
楽曲の音符と編集履歴は楽曲ごとのSQLite（`.cusic/project.db`）に保存する。cusic内部でGitは使わない。GUIも持たない。

> **開発中 / experimental**：ソースコードの変換・MIDI往復テストはCIで検証するが、Cubase / Piapro Studio V4X / Synthesizer V 2 Proの**実機による受け入れテストは未実施**。実制作前にテストプロジェクトとバックアップで確認してね。各ファイル形式の全機能を保持する変換ではない。

## できること

| 入出力 | コマンド | 対応範囲 |
|---|---|---|
| MIDI | `cusic import song.mid` / `cusic export --format midi` | SMF 0/1、固定テンポ・拍子の音符 |
| TuxGuitar | `cusic import song.tg` | **2.x形式（ZIP + content.xml）**の音符・チューニング・簡易タイ |
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

Go 1.23+。SynthVのJSを自分でビルドする場合のみNode.js 22+。

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

## Sunoの音源

Suno Studioから書き出した複数のWAVステムを、音符とは別のアセットとして保存する。

~~~bash
cusic suno import /path/to/suno-stems
cusic audio list
cusic export --format dawproject --out cubase-arrangement.dawproject
~~~

SunoステムはSHA-256を付けて `.cusic/audio/` にコピーし、SQLiteにはファイル位置とWAV解析メタデータのみ記録する。対応はPCM 16/24/32-bit / Float 32/64-bit RIFF WAV（RF64などは未対応）。

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
