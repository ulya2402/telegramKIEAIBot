# Telegram AI Bot Terintegrasi dengan kie.ai

Bot Telegram yang ditulis dengan **Go (Golang)** untuk membuat gambar dan video menggunakan berbagai model AI (Google Nano Banana, GPT-4o, Google Veo, dll) melalui API Kie.ai.

## ✨ Fitur Utama
- 🖼️ **Generate Gambar**: Mendukung berbagai model seperti Nano Banana, Nano Banana Pro, GPT-4o Image.
- 🎥 **Generate Video**: Mendukung Google Veo untuk text-to-video dan image-to-video.
- 🇮🇩 **Multi-Bahasa**: Mendukung Bahasa Indonesia dan Inggris.
- ⚙️ **Kustomisasi**: Atur rasio aspek, resolusi, format, dan jumlah input gambar.
- 🔄 **Image-to-Image**: Upload gambar Anda sendiri sebagai referensi untuk AI.
- ⚡ **Cepat & Ringan**: Dibuat menggunakan Go dan SQLite (tanpa dependensi CGO yang ribet).

## 🛠️ Persiapan
Sebelum memulai, pastikan Anda memiliki:
1. **Telegram Bot Token**: Dapatkan dari [@BotFather](https://t.me/BotFather).
2. **Kie API Key**: Dapatkan API Key dari Kie.ai.
3. **VPS/Server**: Disarankan menggunakan Linux (Ubuntu/Debian).

## 🚀 Cara Install (VPS / Linux)

Ikuti langkah-langkah ini untuk memasang bot di VPS Anda.

### 1. Install Go (Golang)
Jika belum ada Go, install versi terbaru:
```bash
wget https://go.dev/dl/go1.24.0.linux-amd64.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.24.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
go version
```

### 2. Clone Repository
Salin kode bot ini ke server Anda menggunakan git:
```bash
git clone https://github.com/ulya2402/telegramKIEAIBot.git
```
```bash
cd telegramKIEAIBot
```

### 3. Konfigurasi
Buat file `.env` dari contoh yang ada:
```bash
cp .env.example .env
```
```bash
nano .env
```
Isi data berikut di dalam file `.env`:
```ini
TELEGRAM_BOT_TOKEN=123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11
KIE_API_KEY=9DTxxxxxxxxxxxxxxxxxxxx
DB_PATH=./kieAITelegram.db
DEFAULT_LANG=id
```
Simpan dengan `Ctrl+X`, lalu `Y`, lalu `Enter`.

### 4. Build & Jalankan
# Download dependensi
```bash
go mod tidy
```
# Jalankan (Mode Testing)
```bash
go run cmd/bot/main.go
```
Jika berhasil, akan muncul pesan "Bot is now running...". Tekan `Ctrl+C` untuk berhenti.

---

## ⚙️ Menjalankan Bot di Latar Belakang (Auto-Start)

Agar bot tetap jalan meskipun Anda keluar dari VPS, gunakan `systemd`.

1. **Buat Aplikasi**
### Masuk ke folder project
```bash
cd telegramKIEAIBot
```
### Build menjadi file bernama 'kiebot'
```bash
go build -o kiebot cmd/bot/main.go
```
### Beri izin eksekusi
```bash
chmod +x kiebot
```

2. **Menyiapkan Path (Lokasi Folder)**
Sebelum membuat konfigurasi, Anda harus tahu lokasi lengkap folder project Anda saat ini agar systemd bisa menemukannya.

Ketik perintah ini di terminal:
```bash
pwd
```
Contoh output yang mungkin muncul: `/root/telegramKIEAIBot` atau `/home/ubuntu/telegramKIEAIBot`

**Catat hasil output tersebut!** Kita sebut ini sebagai **[LOKASI_FOLDER]**.

**WorkingDirectory** adalah: `[LOKASI_FOLDER]`
**ExecStart adalah:** `[LOKASI_FOLDER]/kiebot`

3. **Buat File Service**
Buat file konfigurasi baru untuk bot:
```bas
sudo nano /etc/systemd/system/aibot.service
```

4. **Isi Konfigurasi**
```TOML
[Unit]
Description=Telegram AI Bot Kie Ai
After=network.target

[Service]
# User VPS (biasanya root, ubuntu, atau nama user Anda)
User=root
Group=root

# Isi dengan hasil command 'pwd' tadi
WorkingDirectory=/root/telegramKIEAIBot

# Isi dengan hasil command 'pwd' ditambah '/kiebot'
ExecStart=/root/telegramKIEAIBot/kiebot

# Restart otomatis jika bot crash
Restart=always
RestartSec=5

# Menyimpan log output
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

5. **Aktifkan & Jalankan Bot**
```bash
sudo systemctl daemon-reload
```
```bash
sudo systemctl enable aibot
```
```bash
sudo systemctl start aibot
```

4. **Cek Status:**
```bash
sudo systemctl status aibot
```
5. **Melihat Log:**
```bash
sudo journalctl -u aibot -f
```

---

## 🎮 Cara Penggunaan (Commands)

- `/start` - Menampilkan pesan selamat datang.
- `/img` - Memilih provider untuk membuat **Gambar**.
- `/vids` - Memilih provider untuk membuat **Video**.
- `/lang` - Mengganti bahasa (Indonesia/Inggris).
- `/cancel` - Membatalkan proses yang sedang berjalan.

## 📝 Konfigurasi Lanjutan (`models.json`)

Anda bisa menambah atau mengubah model AI tanpa mengubah kode program. Edit file `models.json`.

Contoh struktur:
```json
{
  "id": "provider_id",
  "name": "Nama Provider",
  "models": [
    {
      "id": "model_id",
      "name": "Nama Tampilan Model",
      "api_model_id": "nama_model_di_api_kie",
      "description": "Deskripsi singkat.",
      "supported_ops": ["ratio", "resolution"], 
      "ratios": ["1:1", "16:9"],
      "resolutions": ["1K", "4K"]
    }
  ]
}
```
- **supported_ops**: Fitur yang tersedia untuk model tersebut (ratio, format, resolution, image_input).

## 📂 Struktur File
Berikut adalah penjelasan singkat mengenai struktur folder proyek ini:

```
├── cmd/
│   └── bot/
│       └── main.go       # Entry point (Titik awal aplikasi berjalan)
├── internal/
│   ├── api/              # Client untuk menghubungi API eksternal (Kie.ai)
│   ├── bot/              # Logika utama bot (Handler pesan, callback, dll)
│   ├── config/           # Pemuat konfigurasi dari file .env
│   ├── core/             # Logika inti (Registry model, provider)
│   ├── database/         # Koneksi dan operasi database SQLite
│   ├── i18n/             # Sistem bahasa (Internationalization)
│   └── models/           # Struktur data (Structs) untuk JSON & Database
├── locales/              # File JSON untuk terjemahan bahasa (id.json, en.json)
├── .env.example          # Contoh konfigurasi environment
├── go.mod                # Definisi modul dan dependensi Go
└── models.json           # Konfigurasi dinamis untuk model AI
```
