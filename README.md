# Telegram Drive

Telegram Drive is a powerful utility that enables you to create your own cloud storage service using Telegram as the backend.

[![Discord](https://img.shields.io/discord/1142377485737148479?label=discord&logo=discord&style=flat-square&logoColor=white)](https://discord.gg/J2gVAZnHfP)

**Click on icon to join Discord Server for  more advanced configurations for uploads and better support**

## Features

- **UI:** Based on Material You to create nice looking UI themes.
- **Secure:** Your data is secured using robust encryption.
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

- Create the `teldrive.env` file with your variables and start your container.See how to fill env file below.

```sh
docker compose up -d
```
- **Go to http://localhost:8080**

### Use without docker

**Follow Below Steps**

- Download the release binary of Teldrive from the releases section.

- Add same env file as above.
- Now, run the Teldrive executable binary directly.

## Setting up things

If you're locally or remotely hosting, create a file named `teldrive.env` in the root directory and add all the mandatory variables there.
An example of `teldrive.env` file:

```sh
APP_ID=1234
APP_HASH=abc
JWT_SECRET=abc
DATABASE_URL=postgres://<db username>:<db password>@<db host>/<db name>
```
**Generate JWT**
```bash
$ openssl rand -hex 32
```
You can generate secret from [here](https://generate-secret.vercel.app/32).

## Important
  - You can set up a local Postgres instance, but it's not recommended due to backup and data transfer hassles. The recommended approach is to use a free cloud-based Postgres DB like [Neon DB](https://neon.tech/).
  - Default Channel can be selected through UI. Make sure to set it from account settings on first login.
  - Multi Bots Mode is recommended to avoid flood errors and enable maximum download speed, especially if you are using downloaders like IDM and aria2c, which use multiple connections for downloads.
  - To enable multi bots, generate new bot tokens from BotFather and add them through UI on first login.
  - Uploads from UI will be slower due to limitations of the browser. Use modified [Rclone](https://github.com/divyam234/rclone) version for teldrive.
  - Teldrive supports image thumbnail resizing on the fly. To enable this, you have to deploy a separate image resize service from [here](https://github.com/divyam234/image-resize).
  - After deploying this service, add its URL in Teldrive UI settings in the **Resize Host** field.

### Env Variables

| Variable        | Required | Description                                                                               |
| --------------- | -------- | ----------------------------------------------------------------------------------------- |
| `APP_ID`        | Yes      | API ID for your Telegram account, which can be obtained from my.telegram.org.              |
| `APP_HASH`      | Yes      | API HASH for your Telegram account, which can be obtained from my.telegram.org.             |
| `JWT_SECRET`    | Yes      | Used for signing JWT tokens.                                                               |
| `DATABASE_URL`  | Yes      | Connection String obtained from Postgres DB (you can use Neon db as a free alternative for Postgres). |

| Variable           | Default Value | Required | Description                                                                               |
|--------------------|---------------|----------|-------------------------------------------------------------------------------------------|
| `HTTPS`            | false         | NO       | Needed for cross domain setupNeeded for cross domain setup.                        |
| `PORT`             | 8080          | NO       | Change default listen port.                                                      |
| `ALLOWED_USERS`    |               | NO       | Allow certain Telegram usernames, including yours, to access the app.                      |
| `COOKIE_SAME_SITE` | true          | NO       | Needed for cross domain setup.                         |
| `BG_BOTS_LIMIT`    | 5             | NO       | Start at most BG_BOTS_LIMIT no of bots in the background to prevent connection recreation on every request (Default 5). |
| `UPLOAD_RETENTION` | 15            | NO       | No of days to keep incomplete uploads parts in the channel; afterwards, these parts are deleted. |
| `ENCRYPTION_KEY`  |               | NO       | Password for Encrypting files.                                                                  |
| `DEV`              | false         | NO       | DEV mode to enable debug logging.                                         |
| `LOG_SQL`          | false         | NO       | Log SQL queries.                                                          |

> [!WARNING]
> Keep your Password safe once generated teldrive uses same encryption as of rclone internally 
so you don't need to enable crypt in rclone.**Teldrive generates random salt for each file part and saves in database so its more secure than rclone crypt whereas in rclone same salt value  is used  for all files which can be compromised easily**. Enabling crypt in rclone makes UI reduntant so encrypting files in teldrive internally is better way to encrypt files and more secure encryption than rclone.To encrypt files see more about teldrive rclone config.

### For making use of Multi Bots

> [!WARNING]
> Bots will be auto added as admin in channel if you set them from UI if it fails somehow add it manually.For newly logged session you have to wait 20-30 min to add bots to telegram channel.

### Rclone Config Example
```conf
[teldrive]
type = teldrive
api_host = http://localhost:8080 # default host
access_token = #session token obtained from cookies
chunk_size = 500M
upload_concurrency = 4
encrypt_files = false # Enable this to encrypt files make sure ENCRYPTION_KEY env variable is not empty in teldrive instance.
```
**See all options in rclone config command**

[Read Wiki for FAQ](https://github.com/divyam234/teldrive/wiki).

## Best Practices for Using Teldrive

### Dos:

- **Follow Limits:** Adhere to the limits imposed by Telegram servers to avoid account bans and automatic deletion of your channel.

### Don'ts:

- **Data Hoarding:** Avoid excessive data hoarding, as it not only violates Telegram's terms but also leads to unnecessary wastage of storage space.

- **No Netflix Clones:** Refrain from using this service to create your own Netflix-like platform. Saving large amounts of movies or content for personal media servers is discouraged, as it goes against the intended use of Telegram.

### Additional Recommendations:

- **Responsible Storage:** Be mindful of the content you store on Telegram. Utilize storage efficiently and only keep data that serves a purpose.

- **Respect Terms of Service:** Familiarize yourself with and adhere to the terms of service provided by Telegram to ensure a positive and sustainable usage experience.

By following these guidelines, you contribute to the responsible and effective use of Telegram, maintaining a fair and equitable environment for all users.


## Contributing

Feel free to contribute to this project if you have any further ideas.

## Donate

If you like this project small contribution would be appreciated [Paypal](https://paypal.me/redux234).
