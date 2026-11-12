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

macOS, Linux ya da WSL ile Windows'ta (bir WSL terminali açın; WSL'siz Windows için
[Windows](#windows)) tek satır:

```sh
curl -fsSL https://raw.githubusercontent.com/hamzayogurtcuoglu/i-mux/main/install.sh | sh
```

Betik işletim sisteminize ve işlemcinize uygun sürümü indirir, sağlama toplamını doğrular, `imux`'u
`~/.local/bin` dizinine kurar, bu dizini kabuğunuzun PATH'ine ekler (zsh, bash ya da fish), tmux
eksikse (3.2+; tıklanabilir agent çubuğu için 3.4+) kurmayı önerir ve kenar panelin simgelerinin
istediği Nerd Font'u önerir (aşağıda). Ardından yeni bir terminal açın. Seçenekler `sh -s --`
sonrasına yazılır, örneğin tmux'u ve fontu sormadan kurmak için `... | sh -s -- --yes`, ya da
`--version v0.1.0`, `--prefix DİZİN`, `--no-modify-path`, `--no-font`.

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

## Windows

> **Öneri:** Bugün Windows'ta en kararlı günlük kullanım için i-mux'u **WSL 2** içinde yukarıdaki tek
> satırlık `install.sh` komutuyla çalıştırmanızı öneririz (ek kurulum gerektirmeden tam `tmux`
> desteği). Aşağıdaki WSL'siz yerli Windows desteği [psmux](https://github.com/marlocarlo/psmux)
> üzerinde çalışır ve winget'te `3.4.0` sonrası yeni bir sürüm yayınlanana kadar `psmux`'un `main`
> dalından derlenmesini (`scripts\build-psmux.ps1`) gerektirir.

i-mux Windows'ta WSL olmadan, doğrudan da çalışır: tmux'un komutlarını konuşan yerli bir Windows
çoklayıcısı olan [psmux](https://github.com/marlocarlo/psmux) üzerinde. PowerShell'de:

```powershell
irm https://raw.githubusercontent.com/hamzayogurtcuoglu/i-mux/main/install.ps1 | iex
```

Betik psmux eksikse winget ile kurar (`winget install --id marlocarlo.psmux`), işlemcinize uygun
(amd64 ya da arm64) sürüm zip'ini indirir, sağlama toplamını doğrular, `imux.exe`'yi
`%LOCALAPPDATA%\Programs\imux` klasörüne koyar, bu klasörü kullanıcı PATH'inize ekler ve Nerd
Font'u önerir. Ardından yeni bir terminal açın. Dosya olarak çalıştırıldığında seçenek alır
(`.\install.ps1 -Yes`, `-Version v0.1.0`, `-InstallDir KLASÖR`, `-NoModifyPath`, `-NoFont`);
`iex`'e aktarırken önce `IMUX_YES=1`, `IMUX_VERSION` gibi değişkenleri ayarlayın. Go kurulu bir
kaynak kopyasında `.\install.ps1 -FromSource .` derleyip kurar; `scripts\build-windows.ps1` (ya da
`make build-windows`) yalnızca `bin\imux.exe`'yi derler.

**psmux sürümü:** Güncel psmux sürümü 3.4.0, bir pane'e Ctrl+C ya da fare tıklaması gittikten sonra
i-mux'un tuş bağlamalarını ve tıklamalarını çalıştırmıyor, durum çubuğu da tıklanamıyor. İkisi de
psmux'un ana dalında düzeltildi ama henüz sürüm çıkmadı. O zamana kadar `scripts\build-psmux.ps1`
ile derleyin (`winget install --id Rustlang.Rustup -e` ve
`winget install --id MartinStorsjo.LLVM-MinGW.UCRT -e` gerekir, yönetici yetkisi gerekmez):
`bin\psmux.exe` üretir; `install.ps1 -FromSource .` onu `imux.exe`'nin yanına kurar ve i-mux önce
oraya bakar. 3.4.0 kullanıldığı sürece `imux doctor` uyarır.

[Windows Terminal](https://aka.ms/terminal) kullanın (`winget install Microsoft.WindowsTerminal`):
eski konsol kenar panelin renklerini ve simgelerini kötü çizer. `imux font install` fontu sizin
için kurar (`%LOCALAPPDATA%\Microsoft\Windows\Fonts` içine) ve Windows Terminal'in ve VS Code'un
fontunu ayarlar. macOS ve Linux'tan farklar:

- **Bölmelerdeki kabuk PowerShell'dir** (kuruluysa pwsh, değilse Windows PowerShell); i-mux'un
  tuşlarının ve menülerinin çalıştırdığı komutlar da öyle.
- **Dosyalar:** "Göster" Gezgin'i açar, silmek Geri Dönüşüm Kutusu'na taşır.
- **Ctrl+V:** Windows Terminal onu kendi yapıştırması için tutar; resimleri **Alt+V** ile
  yapıştırın.
- **Agent CLI'lar** Windows komutlarıyla kurulur: Claude Code
  `irm https://claude.ai/install.ps1 | iex` (ya da npm), Antigravity CLI
  `irm https://antigravity.google/cli/install.ps1 | iex`, Copilot, Codex ve OpenCode npm ile
  (`npm i -g @github/copilot`, `@openai/codex`, `opencode-ai`). `imux doctor` hepsini gösterir.
- **Güncellemeler:** `imux update` sürümün `install.ps1`'ini çalıştırır. Windows çalışan bir
  `.exe`'nin üzerine yazamaz; eskisi `imux.exe.old` olarak durur ve sonraki bir açılışta silinir.
  Sürümler ve güncellemeler psmux'u `winget upgrade --id marlocarlo.psmux` ile listeler; bunu
  i-mux kapalıyken çalıştırın (önce `psmux -L imux kill-server`).

Kaldırmak için `%LOCALAPPDATA%\Programs\imux` ve `%USERPROFILE%\.imux` klasörlerini silin, klasörü
kullanıcı PATH'inizden çıkarın ve `winget uninstall marlocarlo.psmux` çalıştırın. Uyarlamanın
nasıl çalıştığı: [docs/WINDOWS.md](docs/WINDOWS.md) (İngilizce).

## Kullanım

```sh
cd ~/code/projem
imux            # kurulu ilk agent'ı başlatır (ya da: imux claude)
```

Terminalin bir [Nerd Font](https://www.nerdfonts.com) kullanıyorsa şerit editördeki simgeleri
gösterir, yoksa emojileri. **`imux font install`** (Ayarlar'da da var, kurulum betiği de önerir)
JetBrainsMono Nerd Font'u indirir, sürümün sağlama toplamıyla denetler, yalnızca senin için kurar
(yönetici izni gerekmez) ve Windows Terminal'in ve VS Code'un terminal fontunu ona ayarlar; eski
ayarları `settings.json.imux-backup` içinde saklar. WSL'de bunların hepsini metni çizen terminalin
çalıştığı Windows tarafında yapar. Sonra terminali yeniden başlat; diğer terminallerde (Terminal.app,
iTerm2, GNOME Terminal ...) ayarlarından "JetBrainsMono Nerd Font"u seç. `imux font` neyin yerinde
olduğunu gösterir. Açık görünümün simgesine tıklamak, editördeki gibi şeridin yanındaki paneli
kapatır ve yeri ajana bırakır; herhangi bir görünümün simgesine tıklamak onu yeniden açar.
Panelin kenarını sürükleyerek onu genişletir ya da daraltırsın: her sekme bu genişliğe geçer ve
i-mux onu hatırlar (`tmux.sidebar_width`).

Şeritteki **🔧 Ayar** (ya da `imux settings`, ya da kenar çubuğunda `,`) Ayarlar'ı açar:
düzenleyicin, bildirimler, simgeler, commit mesajlarını kimin yazacağı, geçişten sonra yeni ajanın ne
yapacağı, limitte ne olacağı ve ayar dosyası. Bir satıra Enter ya da tıklama, değerlerinin menüsünü
açar (● şu ankini gösterir); birini seç ya da numarasına bas. Oradaki **Sürümler ve güncellemeler** (ya da `imux
versions`) i-mux'u, tmux'u ve her agent CLI'ını sürümü ve onu güncelleyen komutla listeler; harfine
basınca komut orada çalışır. Eski bir tmux'ta neyin eksik olduğunu ve sisteminde yenisini nasıl
alacağını da söyler. **Editörde yapay zekâ tamamlama** VS Code'daki Copilot
gibi çalışır: oradan **Düzenleyici → Neovim + Copilot**'u seçince düzenlediğin dosyalar GitHub'ın
Copilot eklentili Neovim'inde açılır (Tab
öneriyi kabul eder, Ctrl+S kaydeder, Ctrl+Q çıkar). Eksikleri kurar (Homebrew ile Neovim, eklenti) ve
GitHub Copilot'a bir kez giriş yaptırır.

**Canlı web sayfaları:** gezginde bir HTML dosyasına tıklayınca nasıl açılacağını sorar: **Tarayıcıda
aç** (kendi penceresinde, sekme ya da adres çubuğu olmadan; Chrome'un ya da Edge'in; Windows'ta ve
WSL'de Windows'unki), **Yeni sekmede aç**, **Düzenle** ya da **Görüntüle**. Sayfayı i-mux `localhost` üzerinden sunar;
bir ajan onu ya da kullandığı bir dosyayı yeniden yazınca sayfa kendini yeniler. Başka bir sayfaya
tıklayınca o pencerede açılır. Agent'lar yanındaki **+** → **🌐 Web sayfası…** herhangi bir adresi
açar (`localhost:5173`, ya da sadece `5173`). Kurulacak bir şey yok.
VS Code'un terminalinden (tünel üzerinden de) sayfalar VS Code'un çalıştığı yerde açılır; düz SSH'de
adres panoya gelir. `imux web` sunulanları listeler, `imux web open <dosya|url>` bir sayfa açar,
`imux web stop` durdurur.

**Bildirimler:** bir ajan işini bitirince (en az 10 saniye çalıştıktan sonra) ya da sana bir şey
sorunca, o sırada ona bakmıyorsan bildirim gelir: macOS'un bildirim merkezi, Linux masaüstününki
(`notify-send`), SSH'ta ya da sunucuda ise terminalinin kendi bildirimi (iTerm2, Ghostty, WezTerm,
kitty, GNOME Terminal …). Terminal kapalıyken de çalışır. Bildirimi ve sesini Ayarlar'dan kapatabilir,
`[notify] command` ile istediğin yere (örneğin telefonuna) gönderebilirsin.

**Resim yapıştırma:** bir ekran görüntüsü (ya da Finder'da / dosya yöneticinde resim dosyaları)
kopyala ve herhangi bir ajanın panelinde **Ctrl+V**'ye bas; istediğin kadar tekrarlayabilirsin.
i-mux resmi `~/.imux/images/` altına kaydeder ve yolunu ajana, sürüklenip bırakılmış bir dosya
gibi verir (Antigravity `@yol` alır). **Alt+V** de aynısını yapar; Ctrl+V'yi terminalin
kendisi yakalıyorsa (Windows Terminal) onu kullan. Panoda resim yoksa Ctrl+V her
zamanki gibi ajana gider. macOS panosunu, Wayland'inkini (`wl-clipboard`), X11'inkini (`xclip`), WSL'de
ise Windows panosunu okur: Windows'ta kopyala, WSL'de yapıştır. SSH'ta pano öbür makinede kaldığı
için orada çalışmaz. `imux doctor` neyi kullandığını söyler; Ayarlar'dan kapatılabilir.

**OpenCode**, OpenCode'da tanımladığın herhangi bir model uç noktasıyla çalışır (NVIDIA'nın ya da
Groq'un ücretsiz katmanları gibi OpenAI uyumlu API'ler, Anthropic, kendi makinende Ollama ...):
diğerleri limite takılınca devam etmek için kullanışlı bir yer. i-mux OpenCode 2 ister:
`brew install opencode` onu kurar. Kurulum betiği ve npm hâlâ OpenCode 1 kurar; onlarla kurduysan
ardından `opencode upgrade --major` çalıştır. `imux doctor` çalışabildiğini kontrol eder. Anahtarı OpenCode'un içinde `/connect` ile ekle (NVIDIA
ya da Groq'u seç, build.nvidia.com ya da console.groq.com'dan aldığın anahtarı yapıştır; hesap
gerekmez), sonra `/models` ile modeli seç.

Dosyalar editöründe açılır: Ayarlar → **Düzenleyici** micro, nano ya da Neovim'i seçer (`imux
editor use <editör>`); **Otomatik**'te micro, `$VISUAL`, `$EDITOR`, nano, vim. Kendi editörün yoksa,
açtığın ilk dosya micro'yu senin için getirmeyi önerir (bir kez, 5 MB; ayrıca Ayarlar →
**Düzenleyici** → micro ya da `imux editor install`): fare, Ctrl+S / Ctrl+Q,
Ctrl+C / Ctrl+V, renkli kod ve kaydırma çubuğu. nano da sağ kenarında nerede olduğunu gösterir.
Fare tekerleği her yerde kaydırır.

`imux`'u çalıştırınca hangi projeyi açacağını sorar: adının birkaç harfini yaz. (Açık bir projenin
klasöründeysen doğrudan ona gider; başka bir klasörde, açık ya da geri yüklenecek bir şey yoksa o
klasörü açar.) Bilgisayar yeniden başladıysa ya da i-mux'u kapattıysan en üstte **Son oturumu geri
yükle** gelir ve açık olan bütün projeleri geri getirir; sonra **bu klasör**, sonra **Son
kullanılanlar**: bıraktığın gibi açılırlar, aynı sekmeler, adlar ve agent'lar, her agent kendi
sohbetine döner.

**Çıkmak:** sekme çubuğunun sağındaki **⏻ Çık** → **Terminale dön** seni kabuğuna döndürür, agent'lar
çalışmaya devam eder (`imux` seni onlara geri götürür); **i-mux'u kapat, tüm agent'ları durdur…** bir
kez sorup hepsini kapatır (kabuktan `imux quit`).

Soldaki **kenar panel** bir kod editörününki gibi çalışır: en soldaki simgeler (altlarında adları)
görünümü değiştirir.

| Simge | Görünüm |
|---|---|
| 🤖 | **Agent'lar:** projelerin (branch'iyle) ve içlerinde çalışan bütün agent'lar, her birinin ne yaptığıyla: çalışırken dönen bir işaret (agent'ın kendisi çalıştığını söylerken, Claude Code'un "esc to interrupt" satırı gibi), sana bir şey sorduğunda **Sıra sende**, boştayken halka ve bunun ne kadar önce olduğu; **Agent'lar** başlığı çalışan agent'ları sayar, 🤖 simgesindeki mavi rozet de; her görünümde gözünün önündedir |
| 📁 | **Dosyalar:** **Ctrl+P** (ya da başlığındaki 📄) editördeki gibi projede dosya bulur: adından harfler yaz (`sbrnd`, `sidebar/render.go`'yu bulur; `ad:42` 42. satırda açar), Enter açar. Projenin dosyaları, editördeki gibi her biri türünün simgesiyle (Nerd Font varsa Seti'ninkiler, yoksa emoji), açık ya da kapalı klasörler ve düzeyleri gösteren girinti çizgileriyle; klasöre tıklayınca açılır, dosyaya tıklayınca düzenlenir (HTML dosyası tarayıcında canlı açılır, aşağıda); değişen dosyalar renklidir. **Sağ tık** editördeki menüyü açar: Yeni dosya / Yeni klasör, Yeniden adlandır, Sil (Çöp Sepeti'ne), Kes / Kopyala / Yapıştır, Yolu / Göreli yolu kopyala, Finder'da göster, Terminalde aç, Yanda aç, Klasörde ara, Karşılaştır, Ajana ekle; HTML ve SVG dosyalarında Önizlemeyi Aç. İmlecin üzerindeki şey parlar; seçili dosya işaretli kalır |
| 🔍 | **Arama:** projenin bütün dosyalarında, yazdıkça sonuçlar, dosya dosya, ikonu ve eşleşme sayısıyla (dosyaya tıkla: daralt; hepsi için **⊟ Hepsini kapat** ya da Tab; çok dosya varsa kapalı gelir); Enter dosyayı o satırda açar. Uzun listelerde, diff'lerde ve dosyalarda tıklanıp sürüklenebilen ince bir kaydırma çubuğu var |
| 🔀 | **Git:** VS Code'un kaynak kontrolü, bütün işlemleriyle. **⋯** menüsünü açar: Pull, Push, Clone, Checkout to, Fetch, Git çıktısı ve Commit (amend, signed off, geri al), Changes, Pull/Push, Branch (merge, rebase, oluştur, yeniden adlandır, sil, yayınla), Remote, Stash, Tags ve Worktrees alt menüleri, bir de View & Sort (liste ya da ağaç; ada, yola ya da duruma göre). Dal, etiket, stash ve remote'lar yazdıkça süzülen bir Quick Pick'ten seçilir. Merge, hazırlanan ve hazırlanmayan değişiklikler (her dosyanın önce adı, sonra gri klasörü), her dosyada, klasörde ve hepsi için hazırla, geri al ve at (önce sorar); **Commit…** yanında ▾ (Commit & Push, Commit & Sync, Amend), rebase sürerken Continue ve **✨**: ajanın (ya da `git.commit_agent`, örneğin ücretsiz Groq anahtarıyla OpenCode) değişikliklerden mesajı yazar, sen commit eder, düzenler ya da başkasını istersin; ↑↓ sayılarıyla pull / push / senkron, yeni dal için Publish; dala tıklayınca Checkout to…; dosyaya ya da commit'e sağ tıklayınca menüsü (.gitignore'a ekle, Cherry Pick, Revert, Etiket oluştur…); dal ve etiket etiketli grafik olarak geçmiş. Dosyaya tıklayınca dosyanın tamamı değişiklikler vurgulu açılır, pencere genişse yan yana |
| 🧩 | **Skill'ler:** projenin `<ad>/SKILL.md` skill'leri; beş agent'tan hangilerinin onu gördüğünü (`✻▲◉◎▣`) ve üstbilgisinde sorun varsa `⚠` uyarısını gösterir. Skill'ler `.agents/skills/` altında tutulur ve beş CLI'da birden çalışması için `.claude/skills/` içine otomatik bağlanır. **✨ Yeni skill…** kimin yazacağını (yeni bir bölmede bir agent, bu bölmedeki agent ya da boş bir `SKILL.md` ile sen) ve skill'in ne yapacağını soran bir pencere açar; agent i-mux'un skill yazma rehberiyle başlar, depoyu inceleyip yazar; bir skill'deki **✨** düğmesi (ya da sağ tık) menüsünü açar: Aç, Görüntüle, Agent'a ekle, Agent ile iyileştir, Tüm agent'larla paylaş, Sil |
| 📊 · ❓ | kullanım limitleri · bütün tuşlar |

| Kenar panel / tuş | Ne yapar |
|---|---|
| bir projeye tıkla · **Projeler** yanındaki **+** (ya da `o`) | o projeye geç (projeyi **kapatmak** için mevcut projeye tıkla ya da herhangi birine sağ tıkla) · başka proje aç: adının birkaç harfini yaz, Enter |
| bir agent'a sağ tıkla (ya da seçili olana tıkla) | menüsü: **Yeniden adlandır…** (sohbet başka CLI'ya geçse de ad kalır, Geçmiş de konuşmayı bu adla listeler) ya da **Agent'ı kapat…** |
| bir agent'a tıkla · **Agent'lar** yanındaki **+** | ona git (hangi projede olursa) · yeni agent ya da düz **Terminal** aç, sonra yerini seç: sağda, solda, altta, üstte ya da yeni sekmede (sağa / aşağı için `prefix v` / `prefix -`); ya da **🌐 Web sayfası…** |
| **Bu sohbete şurada devam et** altındaki bir agent'a tıkla | etkin pane o agent'ta, konuşmayla birlikte devam eder (kurulu olmayan biri, onu kuran komutu ve **Şimdi kur** düğmesini gösteren küçük bir pencere açar) |
| bir bölüm başlığı (PROJELER, AGENT'LAR) | bölümü kapatır / açar |
| **⧉ Geçmiş** (ya da `prefix S`) · şeritteki **◔** | eski konuşmalar, ad verilenler adıyla (konuşmanın **Yeniden adlandır…** öğesi ad verir; devam edince agent adını geri alır) (📌 Sabitle ya da kaldır…, `p`, birini en üstte tutar; **🔍 Metinlerinde ara…**, `/`, bir konuşmayı içinde geçen herhangi bir şeyle bulur) · plan kullanımı (Claude ve Codex: 5 saatlik ve haftalık %; Copilot: aylık) |
| en üstteki, terminalin üstündeki **sekme çubuğu** · **+** (ya da `prefix c`) · **×** · **⏻ Çık** | her iş için bir sekme, agent'ları ve durumuyla; tıklayınca geçersin · seçtiğin agent ile yeni sekme (`prefix ,` sekmeye ad verir) · sekmeyi kapat · terminale dön ya da i-mux'u kapat |
| projenin son agent'ını kapat | proje açık kalır ve **Agent yok** der: başlatmak için orada bir agent'ın tuşuna bas (ya da tıkla) veya AGENT'LAR yanındaki **+** |
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
yalnızca aradaki gelişmeleri anlatır. Komut satırı için `imux help` (`switch`, `split`, `ls`, `versions`,
`limits`, `sessions`, `handoff --print`, `editor`, `quit`).

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
[Agent veri yolu](docs/AGENT-BUS.md) · [Windows](docs/WINDOWS.md) · [Sorun giderme](docs/TROUBLESHOOTING.md) ·
[Mimari](ARCHITECTURE.md) · [Katkı](CONTRIBUTING.md) · [English](README.md)

## Lisans

[MIT](LICENSE)
