![image](https://github.com/user-attachments/assets/5b849dc3-b61c-4e85-ad1a-53f27b040650)


# VN007Go


This Auto restart the VN007 Router if the 5G Frequency is missing. 


Due to a bug in telco it will keep charge your SIM data balance if although you are subscribed to Unli 5G promo. Normally it should disconnect automatically if **Network Mode** is set to **5G NSA Only** and prevent unwanted data charges however sometimes it doesnt. 

You can run this in your **Desktop Terminal** or **Android Termux**

Note: This will also make sure your modem will reconnect to 5G soon as possible in areas with problematic cell receptions.

## Requirements
- Install Go Lang: https://go.dev/doc/install
- clone this repo: https://docs.github.com/en/repositories/creating-and-managing-repositories/cloning-a-repository
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

