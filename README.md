![image](https://github.com/user-attachments/assets/38a34a10-1e60-4c94-bf01-a142ce31cc48)


# VN007Go


This Auto restart the VN007 Router if the 5G Frequency is missing. 


Due to a bug in the telco, it will charge your SIM data balance even if you are subscribed to Unli 5G promo. Normally it should disconnect automatically if **Network Mode** is set to **5G NSA Only** and prevent unwanted data charges however sometimes it doesn't. 

You can run this in your **Desktop Terminal** or **Android Termux**

Note: This will also ensure your modem will reconnect to 5G as soon as possible in areas with problematic cell receptions.

## Requirements
- Install Go Lang: [https://go.dev/doc/install](https://go.dev/doc/install)
- clone this repo: [https://github.com/rpfilomeno/vn007go](https://github.com/rpfilomeno/vn007go) ([how-to](https://docs.github.com/en/repositories/creating-and-managing-repositories/cloning-a-repository))
- copy the file named `.env.sample` to `.env`
- edit your `.env` file to match your router setup
- download dependecies
```bash
go mod download
```
- run the application
```bash
go run .
```
- optionally build an executable binary (vn007go.exe) based on your system.
```bash
go build .
```
- to use the  executable binary (vn007go.exe) makes sure your .env file is on the same folder

## Pre-compiled download
- [Windows 64-bit release](https://github.com/rpfilomeno/vn007go/releases/tag/release)
- Download the [.env.sample config file](https://raw.githubusercontent.com/rpfilomeno/vn007go/refs/heads/main/.env.sample) then edit and rename it to `.env` for use with this release.
- Edit your `.env` based on your router settings. You can find valuesof  **SESSION_ID** and **PASSWORD_HASH** hash using [Chrome's Developer Tools](https://developer.chrome.com/docs/devtools) during login.
![image](https://github.com/user-attachments/assets/867e7317-6cfd-4675-a840-1ae5b825f44e)


