FROM scratch
COPY teldrive /teldrive
EXPOSE 8080

# --- 数据库配置 ---
ENV DB_DATA_SOURCE="postgresql://bxjlxw1314:Jt2LTxXesDWR9FZpSMVx4Gybb5daA5FB@dpg-d0njnc6uk2gs73c2c0mg-a.oregon-postgres.render.com/teldrive_doa2"

# --- JWT 配置 ---
ENV JWT_SECRET="f8d27c11e14b8215134fd758b4d248c1cc49beaccf6ee544f02e16e0efd8efa6"

# --- Telegram 主配置 ---
ENV TG_APP_ID="21062778"
ENV TG_APP_HASH="55997f52d4f302f89dd44195a9725924"

# --- Telegram 上传配置 ---
ENV TG_UPLOADS_ENCRYPTION_KEY="DivisionOverflowResultAnnoyanceThreadCalculationShillingAdventureElectricReputation8"
ENV TG_UPLOADS_MAX_RETRIES="10"
ENV TG_UPLOADS_RETENTION="7d"
ENV TG_UPLOADS_THREADS="8"

ENTRYPOINT ["/teldrive","run","--tg-storage-file","/storage.db"]
