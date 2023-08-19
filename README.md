<h1 align="center"> Fast Telegram Drive</h1>

[![Discord](https://img.shields.io/discord/1142377485737148479?label=discord&logo=discord&style=flat-square&logoColor=white)](https://discord.gg/hfTUKGU2C6)

 
<details open="open">
  <summary>Table of Contents</summary>
  <ol>
    <li>
      <a href="#how-to-make-your-own">How to make your own</a>
      <ul>
        <li><a href="#deploy-using-docker-compose">Deploy using docker-compose</a></li>
       <li><a href="#deploy-without-docker-compose">Deploy without docker-compose</a></li>
      </ul>
    </li>
    <li><a href="#setting-up-things">Setting up things</a></li>
    <ul>
      <li><a href="#mandatory-vars">Mandatory Vars</a></li>
      <li><a href="#optional-vars">Optional Vars</a></li>
    </ul>
  </ol>
</details>

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

- Create the `.env` file with your variables and start your container.
- **If you are deploying without https replace nginx.conf with  nginx_nossl.conf
in docker-compose.yml**.

**It should look like this below if you are not using https.**
```yml
volumes:
      - ./nginx_nossl.conf:/etc/nginx/conf.d/default.conf
ports:
    - 8000:8000
```
```sh
docker compose up -d
```

### Deploy without docker-compose(Not working will be fixed later)
First clone the repository
```sh
git clone https://github.com/divyam234/teldrive

```
**Follow Below Steps**

- Fork UI Repo given above and Deploy it to Vercel.
- Download release binary of teldrive from releases section.
- .env file will be same as mentioned above and additionally set variables mentioned below.As vercel app is hosted on https so we need local server also on https so that cookies works.
```shell
HTTPS=true
COOKIE_SAME_SITE=false
```
- Generate https cert and key  for localhost using mkcert and put these in sslcerts directory where executable is present.

- If you are using windows make sure to add cert as trusted using mkcert or manually.(You can see mkcert cli how to add that) 

- Rename generated cert and key as cert.pem and key.pem respectively.

- Now run the teldrive executable from releases.

- Finally change API URL from UI deployed on vercel to https://localhost:8080 in settings.

## Setting up things

If you're locally or remotely hosting, create a file named `.env` in the root directory and add all the variables there.
An example of `.env` file:

```sh
APP_ID=1234
APP_HASH=abc
CHANNEL_ID=1234
HTTPS=false
COOKIE_SAME_SITE=true
JWT_SECRET=abc
DATABASE_URL=abc
TG_CLIENT_DEVICE_MODEL="Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36 Edg/115.0.1901.203" # Any valid  browser user agent here
TG_CLIENT_SYSTEM_VERSION=Win32
TG_CLIENT_APP_VERSION=2.1.9 K
TG_CLIENT_LANG_CODE=en
TG_CLIENT_SYSTEM_LANG_CODE=en
TG_CLIENT_LANG_PACK=webk
MULTI_CLIENT=false
MULTI_TOKEN1=55838383:yourfirstmulticlientbottokenhere
MULTI_TOKEN2=55838383:yoursecondmulticlientbottokenhere
MULTI_TOKEN3=55838383:yourthirdmulticlientbottokenhere
```
According to [Telegram TOS](https://core.telegram.org/api/obtaining_api_id#using-the-api-id): *all accounts that sign up or log in using unofficial Telegram API clients are automatically put under observation to avoid violations of the Terms of Service.So you can use APP_ID and APP_HASH from official K Telegram webclient from [here](https://github.com/morethanwords/tweb/blob/464bc4e76ff6417c7d996cca50c430d89d5d8175/src/config/app.ts#L36)*

**Use strong JWT secret instead of pure guessable string.You can use openssl to generate it.**

```bash
$ openssl rand -base64 32
```


**Multi Client Mode is recommended way to avoid flood errors and to enable max download speed if you are using downloaders like IDM and aria2c which use multiple connections to download files.**
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
> You can spin up local postgres instance but its not recommended as there is lot of hassle in backup and transfering data.Recommended way is to use any free cloud postgres DB.I will recommend to use [Neon DB](https://neon.tech/).

## Contributing

Feel free to contribute to this project if you have any further ideas.


