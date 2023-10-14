# Telegram Drive

Telegram Drive is a powerful utility that enables you to create your own cloud storage service using Telegram as the backend.


[![Discord](https://img.shields.io/discord/1142377485737148479?label=discord&logo=discord&style=flat-square&logoColor=white)](https://discord.gg/J2gVAZnHfP) 

**Click on icon to join Discord Server for better support**

 
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
- **Secure:** Your data is secured using Telegram's robust encryption.
- **Flexible Deployment:** Use Docker Compose or deploy without Docker.
## Demo

![demo](./public/demo.png)

[UI Repo ](https://github.com/divyam234/teldrive-ui)

### Deploy using docker-compose
First clone the repository
```sh
git clone https://github.com/divyam234/teldrive
cd teldrive
touch teldrive.db
```


**Follow Below Steps**

- Create the `teldrive.env`  file with your variables and start your container.

```sh
docker compose up -d
```
- **Go to http://localhost:8080**
- **Uploads from UI will be slower due to limitations of browser use [Teldrive Uploader](https://github.com/divyam234/teldrive-upload) for faster uploads.Make sure to use Multi Bots mode if you are using uploader.**

- **If you intend to share download links with others, ensure that you enable multi bots mode with bots.**

### Use without docker

**Follow Below Steps**

- Download the release binary of Teldrive from the releases section.

- Add same env  file as above.
  
- Now, run the Teldrive executable binary directly.

## Setting up things

If you're locally or remotely hosting, create a file named `teldrive.env`  in the root directory and add all the variables there.
An example of `teldrive.env` file:

```sh
APP_ID=1234
APP_HASH=abc
CHANNEL_ID=1234
HTTPS=false
COOKIE_SAME_SITE=true
JWT_SECRET=abc
DATABASE_URL=abc
RATE_LIMIT=true
LAZY_STREAM_BOTS=false

```
> **Warning**
>Default Channel can be selected through UI make sure to set it from account settings on first login.<br>
>Use strong JWT secret instead of pure guessable string.You can use openssl to generate it.<br>

```bash
$ openssl rand -hex 32
```

**Multi Bots Mode is recommended to avoid flood errors and enable maximum download speed, especially if you are using downloaders like IDM and aria2c which use multiple connections for downloads.**

> **Note**
> What it multi bots feature and what it does? <br>
> This feature shares the Telegram API requests between other bots to avoid getting floodwaited (A kind of rate limiting that Telegram does in the backend to avoid flooding their servers) and to make the server handle more requests. <br>

To enable multi bots, generate new bot tokens from BotFather and add it through UI on first login. 

### Mandatory Vars
Before running the bot, you will need to set up the following mandatory variables:

- `APP_ID` : This is the API ID for your Telegram account, which can be obtained from my.telegram.org.

- `APP_HASH` : This is the API HASH for your Telegram account, which can be obtained from my.telegram.org.

- `JWT_SECRET` : Used for signing jwt tokens

- `DATABASE_URL` : Connection String obtained from Postgres DB (you can use Neon db as free alternative fro postgres)

### Optional Vars
In addition to the mandatory variables, you can also set the following optional variables:
- `HTTPS` : Only needed when frontend is on other domain.
- `PORT` : Change listen port default is 8080
- `ALLOWED_USERS` : Allow certain telegram usernames including yours to access the app.Enter comma seperated telegram usernames here.Its needed when your instance is on public cloud and you want to restrict other people to access you app.
- `COOKIE_SAME_SITE` : Only needed when frontend is on other domain.

- `LAZY_STREAM_BOTS` : If set to true start Bot session and close immediately when stream or download request is over otherwise run bots forever till server stops.

- `BG_BOTS_LIMIT` : If LAZY_STREAM_BOTS is set to false it start atmost BG_BOTS_LIMIT no of bots in background to prevent connection recreation on every request(Default is 10).
### For making use of Multi Bots support

> **Warning**
>Bots will be auto added as admin in channel if you set them from UI if it fails somehow add it manually.
## FAQ

- How to get Postgres DB url ?
> You can set up a local Postgres instance, but it's not recommended due to backup and data transfer hassles. The recommended approach is to use a free cloud-based Postgres DB like [Neon DB](https://neon.tech/).

## Contributing

Feel free to contribute to this project if you have any further ideas.

[Read Wiki Here](https://github.com/divyam234/teldrive/wiki).

