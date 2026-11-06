# ajan

**Aynı yapay zekâ kodlama sohbetini, aynı terminal bölmesinde, tek tıkla ya da tek tuşla başka bir
ajan CLI'ında sürdürün.**

Claude Code ile çalışırken kullanım limitine takıldınız. `Ctrl+\`, ardından `g` tuşuna basın: bölme
Gemini CLI ile yeniden başlar ve Gemini sohbetin özetini alır: ilk istek, görev listesi, dokunulan
dosyalar, depo durumu, son konuşma ve kalınan yer. Hiçbir şeyi yeniden anlatmanız, kopyalayıp
yapıştırmanız ya da komut yazmanız gerekmez.

ajan **Claude Code**, **Gemini CLI**, **GitHub Copilot CLI** ve **OpenAI Codex CLI** ile çalışır. Ajanları
kendi soketindeki [tmux](https://github.com/tmux/tmux) içinde çalıştıran tek ve küçük bir programdır;
kendi tmux ayarlarınıza dokunmaz.

```
┌─ Claude · opus ───────────────────────┬─ Gemini ──────────────────────────────┐
│ > giriş yönlendirme hatasını düzelt    │ > src/auth.ts dosyasını incele         │
│ ● Reading src/auth.ts …                │ ✦ Looking at the handler …             │
└────────────────────────────────────────┴────────────────────────────────────────┘
 0:proj*                                       Claude ⛔15:00 · Gemini ● · 14:32
 ajan │ ▶ Claude │ Gemini │ Codex │ Copilot (kurulu değil) │ ⧉ Oturumlar │ ☰
```

## Özellikler

- **Arayüzden geçiş.** Ajan çubuğunda bir ajana tıklayın ya da bölme menüsünü açıp (`Ctrl+\`,
  `prefix a`, kenarlığa sağ tık) ajanın tuşuna basın. Yeni ajan sohbeti ilk istemi olarak alır.
- **Kullanım limitleri.** Küçük bir izleyici ajan ekranlarındaki limit mesajlarını fark eder.
  Varsayılan olarak "şurada sürdür …?" diye sorar; isterseniz kendisi geçer ya da yalnızca limiti
  gösterir. Limitler ve sıfırlanma saatleri durum çubuğunda görünür; `ajan limits` her ajanın yerel
  kullanımını gösterir.
- **Kaldığınız yere dönüş.** Bölmede daha önce çalışmış bir ajana geri döndüğünüzde ajan kendi
  oturumunu sürdürür ve yalnızca aradaki gelişmeleri öğrenir.
- **Oturum seçici.** `prefix S` projenin tüm ajanlardaki son sohbetlerini listeler; birini kendi
  ajanında ya da başka bir ajanda sürdürün.
- **Yan yana birden çok ajan**, her bölmede ajanın adı ve ajanların birbirine mesaj gönderebildiği
  bir **ajan veri yolu** (`ajan send gemini "src/a.ts dosyasını incele"`).
- **Hafif.** Tuşlarınızla ajanlar arasında ajan'a ait hiçbir şey yoktur; terminal işinin tamamını
  tmux yapar. Sürekli çalışan tek süreç izleyicidir (yaklaşık 10 MB, ~%0 CPU). Her şey yerelde kalır.

## Kurulum

macOS ve Linux (Windows'ta WSL içinde). ajan tmux 3.2 ya da daha yenisini gerektirir; 3.4+ ile ajan
çubuğu tıklanabilir olur.

```sh
brew install hamzayogurtcuoglu/tap/ajan
```

ya da

```sh
curl -fsSL https://raw.githubusercontent.com/hamzayogurtcuoglu/i-mux/main/install.sh | sh
```

Betik indirmenin sağlama toplamını doğrular, `~/.local/bin` dizinine kurar (değiştirmek için
`--prefix`) ve tmux eksikse kurmayı önerir (önce sorar, sudo kullanacaksa söyler).

En az bir ajan CLI'ı kurun, sonra kurulumu denetleyin:

```sh
ajan doctor
```

## Hızlı başlangıç

```sh
cd ~/code/projem
ajan            # kurulu ilk ajanı başlatır (ya da: ajan claude)
```

ajan proje için bir tmux oturumu açar ya da varsa ona bağlanır. İçeride:

| Tuş / eylem | Ne yapar |
|---|---|
| `Ctrl+\` | bölme menüsü: ajan değiştir, oturumlar, yeni bölme, devir belgesini önizle, limitler, kapat |
| `Ctrl+\` sonra `c` / `g` / `p` / `x` | bu bölmeyi Claude / Gemini / Copilot / Codex'e geçir |
| `prefix a` (`Ctrl+b a`) | aynı menü (terminal `Ctrl+\` tuşunu kendine ayırdıysa) |
| `prefix S` | oturum seçici |
| çubukta bir ajana tıklamak | etkin bölmeyi o ajana geçirir |
| bölme kenarlığına sağ tık | o bölmenin menüsü |
| `ajan gemini` (bir bölmede) | Gemini ile yeni bir pencere açar |

Komut satırından: `ajan switch gemini`, `ajan split codex -h`, `ajan ls`, `ajan attach`,
`ajan limits`, `ajan sessions`. tmux olmadan `ajan handoff --from claude --to gemini`, Gemini'yi
devir belgesiyle mevcut terminalde başlatır; `ajan handoff --print` yalnızca belgeyi yazdırır.

## Bir ajan limite takıldığında

`~/.ajan/config.toml` içinde `on_limit` ayarlayın (`ajan config init` açıklamalı bir dosya yazar):

| `on_limit` | Davranış |
|---|---|
| `"ask"` (varsayılan) | bir menü: "Claude limitine ulaştı (15:00 saatinde sıfırlanır): şurada sürdür …" |
| `"auto"` | `order` sırasındaki, kurulu ve limitte olmayan ilk ajana geçer |
| `"off"` | limiti yalnızca durum çubuğunda gösterir |

Arayüz dili `LANG` / `AJAN_LANG` değişkeninden ya da yapılandırmadaki `locale = "tr"` ayarından
gelir. Ajanlara giden devir belgesi her zaman İngilizcedir.

## Gizlilik

- **Yalnızca yerel.** ajan ağ bağlantısı kurmaz, telemetri göndermez, model API'lerini çağırmaz ve
  kimlik bilgisi dosyalarını okumaz. Geçiş yaptığınızda ajanların oturum dosyalarını okur ve
  `git status` / `git diff --stat` çalıştırır.
- **Geçiş, sohbeti diğer sağlayıcıyla paylaşır.** Devir belgesi bir sağlayıcıyla yaptığınız
  sohbetten alıntılar ve depo durumunu içerir; yeni ajan bunu kendi sağlayıcısına gönderir. Bu
  yüzden limitler için varsayılan `ask`'tir ve ajan ilk geçişte bir kez uyarı gösterir.
- **Gizli bilgiler** (özel anahtarlar, bulut ve API belirteçleri, `Authorization` başlıkları)
  devir belgesinden elden geldiğince çıkarılır. Oturumda gizli bilgi geçtiyse `ajan handoff --print`
  ile kontrol edin.
- **Süreç listesi.** Varsayılan olarak devir belgesi komut satırı argümanı olarak verilir; aynı
  makinedeki diğer kullanıcılar onu `ps` ile görebilir. Bunun yerine ajana yapıştırmak için
  `[handoff] inject = "paste"` ayarlayın (`ajan doctor` başka kullanıcılar varsa söyler).
- Devir belgeleri, günlükler ve durum `~/.ajan` içinde özel izinlerle (0700 / 0600) durur; devir
  belgeleri 30 gün sonra silinir.
- Ajan veri yolu, ajanlar arasında istem enjeksiyonu taşıyabilir; bkz.
  [docs/AGENT-BUS.md](docs/AGENT-BUS.md).

## Belgeler (İngilizce)

- [docs/CONFIG.md](docs/CONFIG.md): tüm ayarlar
- [docs/HANDOFF.md](docs/HANDOFF.md): yeni ajanın aldığı belge
- [docs/AGENT-BUS.md](docs/AGENT-BUS.md): ajanların birbiriyle konuşması
- [docs/LIMITS.md](docs/LIMITS.md): `ajan limits` ve JSON çıktısı
- [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md): tuşlar, nvm, terminaller, bulunamayan oturumlar
- [README.md](README.md) (English)

## Lisans

MIT. Bkz. [LICENSE](LICENSE).
