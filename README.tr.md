<h1 align="center">i-mux</h1>

<p align="center">
  <b>Bir yapay zekâ kodlama agent'ında kullanım limitine mi takıldınız? Sohbetin tamamıyla başka bir agent'ta devam edin.</b><br>
  Claude Code · Antigravity CLI · GitHub Copilot CLI · OpenAI Codex CLI · OpenCode
</p>

<p align="center">
  <img src="docs/images/limit.svg" alt="Claude limite takılıyor, i-mux Antigravity ya da Codex'te sürdürmeyi öneriyor" width="880">
</p>

i-mux, agent CLI'larınızı bir terminal çalışma alanında çalıştırır. Biri limite takıldığında ya da
siz istediğinizde tek tuşa basarsınız; aynı bölme başka bir agent ile yeniden başlar. Yeni agent
özetlenmiş bir devir belgesi alır: ilk istek, görev listesi, dokunulan dosyalar, depo durumu ve
işin kaldığı yer. Yeniden anlatmak ya da kopyalayıp yapıştırmak yok.

[tmux](https://github.com/tmux/tmux) üzerinde çalışan tek bir programdır, komutu `imux` (kendi
soketinde; sizin tmux ayarlarınıza dokunmaz). Her şey makinenizde kalır.

## Kurulum

macOS, Linux ya da WSL ile Windows'ta (bir WSL terminali açın) tek satır:

```sh
curl -fsSL https://raw.githubusercontent.com/hamzayogurtcuoglu/i-mux/main/install.sh | sh
```

Betik işletim sisteminize ve işlemcinize uygun sürümü indirir, sağlama toplamını doğrular, `imux`'u
`~/.local/bin` dizinine kurar, bu dizini kabuğunuzun PATH'ine ekler (zsh, bash ya da fish) ve tmux
eksikse (3.2+; tıklanabilir agent çubuğu için 3.4+) kurmayı önerir. Ardından yeni bir terminal açın.
Seçenekler `sh -s --` sonrasına yazılır, örneğin tmux'u sormadan kurmak için `... | sh -s -- --yes`,
ya da `--version v0.1.0`, `--prefix DİZİN`, `--no-modify-path`.

Go 1.27+ ile kendiniz de derleyebilirsiniz:

```sh
go install github.com/hamzayogurtcuoglu/i-mux/cmd/imux@latest
```

Sonra en az bir agent CLI'ı kurun ve kurulumu denetleyin. Google modelleri için
[Antigravity CLI](https://antigravity.google/docs/cli/) (`agy`) kurun:
`curl -fsSL https://antigravity.google/cli/install.sh | bash`. Google kişisel hesaplara Gemini
CLI'da 2026-06-18'de hizmeti kesti; bu yüzden i-mux Gemini CLI yerine Antigravity CLI'ı destekliyor.

```sh
imux doctor
```

Kaldırmak için `~/.local/bin/imux` ve `~/.imux` silin, kabuk profilinize eklenen satırı da
(`# Added by the imux installer` ile işaretli) kaldırın.

## Kullanım

```sh
cd ~/code/projem
imux            # kurulu ilk agent'ı başlatır (ya da: imux claude)
```

Terminalin bir [Nerd Font](https://www.nerdfonts.com) kullanıyorsa şerit editördeki simgeleri
gösterir (`brew install --cask font-jetbrains-mono-nerd-font`, sonra terminalin ayarlarından seç);
yoksa emojileri. Açık görünümün simgesine tıklamak, editördeki gibi şeridin yanındaki paneli
kapatır ve yeri ajana bırakır; herhangi bir görünümün simgesine tıklamak onu yeniden açar.

Şeritteki **🔧 Ayar** (ya da `imux settings`, ya da kenar çubuğunda `,`) Ayarlar'ı açar: editörde
yapay zekâ tamamlama, simgeler, commit mesajlarını kimin yazacağı, geçişten sonra yeni ajanın ne
yapacağı, limitte ne olacağı ve ayar dosyası. **Editörde yapay zekâ tamamlama** VS Code'daki Copilot
gibi çalışır: oradan açınca düzenlediğin dosyalar GitHub'ın Copilot eklentili Neovim'inde açılır (Tab
öneriyi kabul eder, Ctrl+S kaydeder, Ctrl+Q çıkar). Eksikleri kurar (Homebrew ile Neovim, eklenti) ve
GitHub Copilot'a bir kez giriş yaptırır.

**Bildirimler:** bir ajan işini bitirince (en az 10 saniye çalıştıktan sonra) ya da sana bir şey
sorunca, o sırada ona bakmıyorsan bildirim gelir: macOS'un bildirim merkezi, Linux masaüstününki
(`notify-send`), SSH'ta ya da sunucuda ise terminalinin kendi bildirimi (iTerm2, Ghostty, WezTerm,
kitty, GNOME Terminal …). Terminal kapalıyken de çalışır. Bildirimi ve sesini Ayarlar'dan kapatabilir,
`[notify] command` ile istediğin yere (örneğin telefonuna) gönderebilirsin.

**OpenCode**, OpenCode'da tanımladığın herhangi bir model uç noktasıyla çalışır (NVIDIA'nın ya da
Groq'un ücretsiz katmanları gibi OpenAI uyumlu API'ler, Anthropic, kendi makinende Ollama ...):
diğerleri limite takılınca devam etmek için kullanışlı bir yer. `brew install opencode` ile kur;
`imux doctor` çalışabildiğini kontrol eder. Anahtarı OpenCode'un içinde `/connect` ile ekle (NVIDIA
ya da Groq'u seç, build.nvidia.com ya da console.groq.com'dan aldığın anahtarı yapıştır; hesap
gerekmez), sonra `/models` ile modeli seç.

Dosyalar editöründe açılır: config'teki `editor`, yoksa micro (terminalde en kolayı: `brew install
micro`), `$VISUAL`, `$EDITOR`, nano, vim. Fare tekerleği her yerde kaydırır.

`imux`'u ana klasöründe çalıştırırsan hangi projeyi açacağını sorar: adının birkaç harfini yaz.
**Son kullanılanlar** en üstte gelir ve bıraktığın gibi açılır: aynı sekmeler, adlar ve agent'lar,
her agent kendi sohbetine döner. Bilgisayar yeniden başladıysa **Son oturumu geri yükle** açık olan
bütün projeleri geri getirir.

Soldaki **kenar panel** bir kod editörününki gibi çalışır: en soldaki simgeler (altlarında adları)
görünümü değiştirir.

| Simge | Görünüm |
|---|---|
| 🤖 | **Agent'lar:** projelerin (branch, kaydedilmemiş değişiklik) ve içlerinde çalışan bütün agent'lar, her birinin ne yaptığıyla: çalışırken dönen bir işaret, sana bir şey sorduğunda **Sıra sende**, sen başka yerdeyken işini bitirdiyse yeşil nokta, boştayken halka ve bunun ne kadar önce olduğu |
| 📁 | **Dosyalar:** projenin dosyaları; klasöre tıklayınca açılır, dosyaya tıklayınca düzenlenir (sağ tık: görüntüle ya da VS Code'da aç); değişen dosyalar renklidir |
| 🔍 | **Arama:** projenin bütün dosyalarında, yazdıkça sonuçlar, dosya dosya (dosyaya tıkla ya da Tab'a bas: daralt); Enter dosyayı o satırda açar |
| 🔀 | **Git:** editördeki kaynak kontrolü gibi: hazırlanan ve hazırlanmayan değişiklikler, her dosyada ve hepsi için hazırla, geri al ve at (önce sorar); **Commit…** ve yanında **✨**: ajanın (ya da `git.commit_agent`, örneğin ücretsiz Groq anahtarıyla OpenCode) değişikliklerden mesajı yazar, sen commit eder, düzenler ya da başkasını istersin; ↑↓ sayılarıyla pull / push / senkron; dal menüsü (değiştir, yeni dal); geçmiş. Dosyaya tıklayınca dosyanın tamamı değişiklikler vurgulu açılır, pencere genişse yan yana |
| 📊 · ❓ | kullanım limitleri · bütün tuşlar |

| Kenar panel / tuş | Ne yapar |
|---|---|
| bir projeye tıkla · **Projeler** yanındaki **+** (ya da `o`) | o projeye geç (projeyi **kapatmak** için mevcut projeye tıkla ya da herhangi birine sağ tıkla) · başka proje aç: adının birkaç harfini yaz, Enter |
| bir agent'a sağ tıkla (ya da seçili olana tıkla) | menüsü: **Yeniden adlandır…** (sohbet başka CLI'ya geçse de ad kalır) ya da **Agent'ı kapat…** |
| bir agent'a tıkla · **Agent'lar** yanındaki **+** | ona git (hangi projede olursa) · yeni bir pane'de yeni agent ya da düz **Terminal** aç (sağa / aşağı için `prefix v` / `prefix -`) |
| **Bu sohbete şurada devam et** altındaki bir agent'a tıkla | etkin pane o agent'ta, konuşmayla birlikte devam eder |
| bir bölüm başlığı (PROJELER, AGENT'LAR) | bölümü kapatır / açar |
| **⧉ Geçmiş** (ya da `prefix S`) · şeritteki **◔** | eski konuşmalar (📌 Sabitle ya da kaldır…, `p`, birini en üstte tutar) · plan kullanımı (Claude ve Codex: 5 saatlik ve haftalık %; Copilot: aylık) |
| en üstteki **sekme çubuğu** · **+** (ya da `prefix c`) · **×** | her iş için bir sekme, agent'ları ve durumuyla; tıklayınca geçersin · seçtiğin agent ile yeni sekme (`prefix ,` sekmeye ad verir) · sekmeyi kapat |
| `prefix b` | kenar paneli gizle ya da göster |
| `Ctrl+\` (ya da `prefix a`), sonra `c` `a` `p` `x` `e` | pane menüsü; Claude / Antigravity / Copilot / Codex / OpenCode'da sürdür |

`prefix`, `Ctrl+b` demektir: bas, bırak, sonra tuşa bas.

<table>
  <tr>
    <td width="50%"><img src="docs/images/switched.svg" alt="Antigravity oturumu i-mux'un devir belgesiyle sürdürüyor"></td>
    <td width="50%"><img src="docs/images/menu.svg" alt="i-mux bölme menüsü"></td>
  </tr>
  <tr>
    <td align="center">Antigravity, Claude'un kaldığı yerden devam ediyor</td>
    <td align="center">Bölme menüsü (<code>Ctrl+\</code>)</td>
  </tr>
</table>

Geçişte bütün konuşmayı değil, tek bir kısa mesaj görürsün: i-mux konuşmayı `~/.imux/handoffs`
içine kaydeder ve yeni agent'a o dosyayı okumasını söyler. Agent dosyayı okur, bağlamı aldığını
tek cümleyle söyler ve seni bekler: kendiliğinden işe girişmez, boşuna token harcamaz. Belgenin
tamamının mesaj olarak gitmesini istersen `[handoff] deliver = "inline"`, yarım kalan işe hemen
devam etmesini istersen `[handoff] on_arrival = "continue"` yaz.

Bölmede daha önce çalışmış bir agent'a geri dönmek, o agent'ın kendi oturumunu sürdürür ve ona
yalnızca aradaki gelişmeleri anlatır. Komut satırı için `imux help` (`switch`, `split`, `ls`,
`limits`, `sessions`, `handoff --print`).

Arayüz dili `LANG` / `IMUX_LANG` ya da yapılandırmadaki `locale = "tr"` ile seçilir; agent'lara
giden devir belgesi her zaman İngilizcedir.

## Bir agent limite takıldığında

Küçük bir izleyici agent ekranlarındaki limit mesajlarını fark eder ve durum çubuğunda gösterir.
Sonrasında ne olacağını `~/.imux/config.toml` içinde seçin (`imux config init` açıklamalı bir dosya
yazar):

```toml
on_limit = "ask"   # ask (varsayılan) | auto: `order` içindeki sıradaki agent'a geç | off: yalnızca göster
```

## Gizlilik

- Ağ bağlantısı, telemetri ya da API anahtarı yok. i-mux, agent'ların yerel oturum dosyalarını ve
  `git status` çıktısını okur.
- Geçiş, devir belgesini sonraki agent'ın sağlayıcısına gönderir. Gizli bilgiler elden geldiğince
  çıkarılır; `imux handoff --print` ile kontrol edin.
- Devir belgesi komut satırı argümanı olarak verilir; makinedeki diğer kullanıcılar onu `ps` ile
  görebilir. Paylaşılan makinelerde `[handoff] inject = "paste"` ayarlayın.
- Ayrıntılar: [docs/HANDOFF.md](docs/HANDOFF.md) ve [docs/AGENT-BUS.md](docs/AGENT-BUS.md).

## Belgeler (İngilizce)

[Yapılandırma](docs/CONFIG.md) · [Devir belgesi](docs/HANDOFF.md) · [Limitler](docs/LIMITS.md) ·
[Agent veri yolu](docs/AGENT-BUS.md) · [Sorun giderme](docs/TROUBLESHOOTING.md) ·
[Mimari](ARCHITECTURE.md) · [Katkı](CONTRIBUTING.md) · [English](README.md)

## Lisans

[MIT](LICENSE)
