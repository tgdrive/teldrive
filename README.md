# Telegram Drive

Telegram Drive is a powerful utility that enables you to create your own cloud storage service using Telegram as the backend.


[![Discord](https://img.shields.io/discord/1142377485737148479?label=discord&logo=discord&style=flat-square&logoColor=white)](https://discord.gg/hfTUKGU2C6)

 
<details open="open">
  <summary>Table of Contents</summary>
  <ol>
    <li>
      <ul>
      <li>
      <a href="#features">Features</a>
    </li>
        <li><a href="#deploy-using-docker-compose">Deploy using docker-compose</a></li>
       <li><a href="#use-without-docker">Use without docker</a></li>
      </ul>
    </li>
    <li><a href="#setting-up-things">Setting up things</a></li>
    <ul>
      <li><a href="#mandatory-vars">Mandatory Vars</a></li>
      <li><a href="#optional-vars">Optional Vars</a></li>
    </ul>
  </ol>
</details>

## Features

- **UI:** Based on Material You to create nice looking UI themes.
- **Fast Downloads:** Get your files quickly with high-speed downloads.
- **Fast Uploads:** Upload your files faster using multi bots.
- **Multi-Client Support:** Avoid rate limits and maximize download speeds with multiple clients.
- **Secure:** Your data is secured using Telegram's robust encryption.
- **Flexible Deployment:** Use Docker Compose or deploy without Docker.
- **Authentication:** Supports Phone, QR and 2FA login.
- **Rclone:** Supports almost all rclone operations.
## Demo

![demo](./public/demo.png)

[UI Repo ](https://github.com/divyam234/teldrive-ui)

### Deploy using docker-compose
First clone the repository
```sh
git clone https://github.com/divyam234/teldrive
cd teldrive
```


**Follow Below Steps**

- Create the `.env` or `teldrive.env`  file with your variables and start your container.

```sh
docker compose up -d
```
- **Go to http://localhost:8080**
- **Uploads from UI will be slower due to limitations of browser use [Teldrive Uploader](https://github.com/divyam234/teldrive-upload) for faster uploads.Make sure to use Multi Client mode if you are using uploader.**

- **If you intend to share download links with others, ensure that you enable multi-client mode with bots.**

### Use without docker

**Follow Below Steps**

- Download the release binary of Teldrive from the releases section.

- Add same env  file as above.
  
- Now, run the Teldrive executable binary directly.

## Setting up things

If you're locally or remotely hosting, create a file named `.env` or `teldrive.env`  in the root directory and add all the variables there.
An example of `.env` file:

```sh
APP_ID=1234
APP_HASH=abc
CHANNEL_ID=1234
HTTPS=false
COOKIE_SAME_SITE=true
JWT_SECRET=abc
DATABASE_URL=abc
RATE_LIMIT=true
TG_CLIENT_DEVICE_MODEL="Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/116.0" # Any valid  browser user agent here
MULTI_CLIENT=false
MULTI_TOKEN1=""
MULTI_TOKEN2=""
MULTI_TOKEN3=""
```
According to [Telegram TOS](https://core.telegram.org/api/obtaining_api_id#using-the-api-id): *all accounts that sign up or log in using unofficial Telegram API clients are automatically put under observation to avoid violations of the Terms of Service.So you can use APP_ID and APP_HASH from official K Telegram webclient from [here](https://github.com/morethanwords/tweb/blob/464bc4e76ff6417c7d996cca50c430d89d5d8175/src/config/app.ts#L36)*

**Use strong JWT secret instead of pure guessable string.You can use openssl to generate it.**

```bash
$ openssl rand -base64 32
```


**Multi-Client Mode is recommended to avoid flood errors and enable maximum download speed, especially if you are using downloaders like IDM and aria2c which use multiple connections for downloads.**
### Mandatory Vars
Before running the bot, you will need to set up the following mandatory variables:

- `APP_ID` : Use official ones as mentioned above.

- `APP_HASH` : Use official ones as mentioned above.

- `JWT_SECRET` : Used for signing jwt tokens

- `DATABASE_URL` : Connection String obtained from Postgres DB (you can use Neon db as free alternative fro postgres)

- `CHANNEL_ID` :  This is the channel ID for the log channel where app will store files . To obtain a channel ID, create a new telegram channel (public or private), post something in the channel, forward the message to [@missrose_bot](https://telegram.dog/MissRose_bot) and **reply the forwarded message** with the /id command. Copy the forwarded channel ID and paste it into the this field.

### Optional Vars
In addition to the mandatory variables, you can also set the following optional variables:
- `HTTPS` : Only needed when frontend is deployed on vercel.
- `PORT` : Change listen port default is 8080
- `ALLOWED_USERS` : Allow certain telegram usernames including yours to access the app.Enter comma seperated telegram usernames here.Its needed when your instance is on public cloud and you want to restrict other people to access you app.
- `COOKIE_SAME_SITE` : Only needed when frontend is deployed on vercel.
- `MULTI_CLIENT` : Enable or Disable Multi Token mode. If true you have pass atleast one Multi Token
- `MULTI_TOKEN[1....]` : Recommended to add atleast 10-12 tokens
### For making use of Multi-Client support

> **Note**
> What it multi-client feature and what it does? <br>
> This feature shares the Telegram API requests between other bots to avoid getting floodwaited (A kind of rate limiting that Telegram does in the backend to avoid flooding their servers) and to make the server handle more requests. <br>

To enable multi-client, generate new bot tokens and add it as your environmental variables with the following key names. 

`MULTI_TOKEN1`: Add your first bot token here.

`MULTI_TOKEN2`: Add your second bot token here.

you may also add as many as bots you want. (max limit is not tested yet)
`MULTI_TOKEN3`, `MULTI_TOKEN4`, etc.

> **Warning**
> Don't forget to add all these bots to the `CHANNEL_ID` as admin for the proper functioning

## FAQ

- How to get Postgres DB url ?
> You can set up a local Postgres instance, but it's not recommended due to backup and data transfer hassles. The recommended approach is to use a free cloud-based Postgres DB like [Neon DB](https://neon.tech/).

## Contributing

Feel free to contribute to this project if you have any further ideas.


