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



