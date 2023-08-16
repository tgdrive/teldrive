<h1 align="center"> Telegram Drive</h1>
 
<details open="open">
  <summary>Table of Contents</summary>
  <ol>
    <li>
      <a href="#how-to-make-your-own">How to make your own</a>
      <ul>
        <li><a href="prerequisites">Prerequisites</a></li>
        <li><a href="#deploy-using-docker-compose">Deploy using docker-compose</a></li>
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
now create the `.env` file with your variables. and start your container:
**If you are deploying without https replace nginx.conf with  nginx_nossl.conf
in docker-compose.yml.It should look like below. Also Replace #DBURL with POSTGRES URL to RUN first time migrations and add  ?search_path=public to postgres url at end so that migrations don't error out.**
```yml
volumes:
      - ./nginx_nossl.conf:/etc/nginx/conf.d/default.conf
ports:
    - 8000:8000
```
```sh
docker compose up -d
```

## Setting up things

If you're locally hosting, create a file named `.env` in the root directory and add all the variables there.
An example of `.env` file:

```sh
API_ID=1234
API_HASH=abc
CHANNEL_ID=1234
JWT_SECRET=abc
DATABASE_URL=abc
MULTI_CLIENT=true # true or false here
MULTI_TOKEN1=55838383:yourfirstmulticlientbottokenhere
MULTI_TOKEN2=55838383:yoursecondmulticlientbottokenhere
MULTI_TOKEN3=55838383:yourthirdmulticlientbottokenhere
```
**Multi Client Mode is recommended way to avoid flood errors and to enable max download speed if you are using downloaders like IDM and aria2c which use multiple connections to download files.**
### Mandatory Vars
Before running the bot, you will need to set up the following mandatory variables:

- `API_ID` : This is the API ID for your Telegram account, which can be obtained from my.telegram.org.

- `API_HASH` : This is the API hash for your Telegram account, which can also be obtained from my.telegram.org.

- `JWT_SECRET` : Used for signing jwt tokens

- `DATABASE_URL` : Connection String obtained from Postgres DB (you can use Neon db as free alternative fro postgres)

- `CHANNEL_ID` :  This is the channel ID for the log channel where app will store files . To obtain a channel ID, create a new telegram channel (public or private), post something in the channel, forward the message to [@missrose_bot](https://telegram.dog/MissRose_bot) and **reply the forwarded message** with the /id command. Copy the forwarded channel ID and paste it into the this field.

### Optional Vars
In addition to the mandatory variables, you can also set the following optional variables:

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


