#!/bin/bash

SERVER_ROOT="/home/FiveM/server"
mkdir -p "$SERVER_ROOT"

LOGFILE="$SERVER_ROOT/setup.log"
ARTIFACT_URL="https://runtime.fivem.net/artifacts/fivem/build_proot_linux/master/7290-a654bcc2adfa27c4e020fc915a1a6343c3b4f921/fx.tar.xz"
PMA_VERSION="5.2.1"
export TERM=xterm
export DEBIAN_FRONTEND=noninteractive

echo "STARTE INSTALLER (SKIP-GRANT METHODE)..." > "$LOGFILE"
echo "ENV CHECK: FORCE_OVERWRITE='$FORCE_OVERWRITE'" >> "$LOGFILE"

safe_json_out() {
    export SUCCESS="$1"
    export ERROR="$2"
    export CODE="$3"
    export PIN="$4"
    export LOGFILE="$5"
    export DB_HOST="$6"
    export DB_NAME="$7"
    export DB_USER="$8"
    export DB_PASS="$9"
    export PMA_URL="${10}"
    export ROOT_USER="${11}"
    export ROOT_PASS="${12}"

    echo "JSON_START"
    python3 -c "
import json, os
raw_log = ''
log_path = os.environ.get('LOGFILE', '')
if log_path and os.path.exists(log_path):
    try:
        with open(log_path, 'r', errors='ignore') as f:
            raw_log = ''.join(f.readlines()[-100:])
    except:
        raw_log = 'Fehler beim Lesen des Logs.'
else:
    raw_log = 'Log nicht gefunden.'

try:
    ip_cmd = os.popen('curl -s ifconfig.me')
    server_ip = ip_cmd.read().strip()
    ip_cmd.close()
except:
    server_ip = '127.0.0.1'

pma_link = os.environ.get('PMA_URL', '')
if pma_link == 'AUTO':
    pma_link = 'http://' + server_ip + '/phpmyadmin'

data = {
    'success': os.environ.get('SUCCESS') == 'true',
    'error': os.environ.get('ERROR', ''),
    'error_code': os.environ.get('CODE', ''),
    'pin': os.environ.get('PIN', ''),
    'tx_url': 'http://' + server_ip + ':40120',
    'pma_url': pma_link,
    'mysql_host': 'localhost',
    'mysql_db': os.environ.get('DB_NAME', ''),
    'mysql_user': os.environ.get('DB_USER', ''),
    'mysql_pass': os.environ.get('DB_PASS', ''),
    'root_user': os.environ.get('ROOT_USER', ''),
    'root_pass': os.environ.get('ROOT_PASS', ''),
    'raw_log': raw_log
}
print(json.dumps(data))
"
    echo "JSON_END"
}

apt-get update -qq > /dev/null
apt-get install -y wget curl screen xz-utils unzip net-tools openssl cron psmisc > /dev/null 2>&1

CLEAN_FORCE=$(echo "$FORCE_OVERWRITE" | xargs)

if [ "$CLEAN_FORCE" == "true" ]; then
    echo ">> FORCE MODE AKTIV! Lösche alles..." >> "$LOGFILE"

    pkill -f fx-server || true
    screen -X -S fxserver quit || true
    sleep 2

    cd /tmp
    if [ -d "$SERVER_ROOT" ]; then rm -rf "$SERVER_ROOT"; fi
    if [ -d "/var/www/html/phpmyadmin" ]; then rm -rf "/var/www/html/phpmyadmin"; fi

    mkdir -p "$SERVER_ROOT"
fi

cd "$SERVER_ROOT" || {
    safe_json_out "false" "Verzeichnisfehler" "FS_ERROR" "" ""
    exit 1
}

if [ "$CLEAN_FORCE" != "true" ]; then
    CONFLICT_SERVER="false"
    CONFLICT_WEB="false"

    if [ -f "run.sh" ]; then CONFLICT_SERVER="true"; fi
    if [ "$INSTALL_MYSQL" == "true" ]; then
        if command -v apache2 >/dev/null 2>&1 || dpkg -s apache2 >/dev/null 2>&1; then
            CONFLICT_WEB="true";
        fi
    fi

    if [ "$CONFLICT_SERVER" == "true" ] && [ "$CONFLICT_WEB" == "true" ]; then
        safe_json_out "false" "Server und Apache2 gefunden." "FULL_SYSTEM_CONFLICT" "" "$LOGFILE"
        exit 0
    elif [ "$CONFLICT_SERVER" == "true" ]; then
        safe_json_out "false" "Ein Server ist bereits installiert." "ALREADY_INSTALLED" "" "$LOGFILE"
        exit 0
    elif [ "$CONFLICT_WEB" == "true" ]; then
        safe_json_out "false" "Apache2 ist bereits installiert." "WEBSERVER_CONFLICT" "" "$LOGFILE"
        exit 0
    fi
fi

DB_NAME_VAL=""
DB_USER_VAL=""
DB_PASS_VAL=""
ROOT_USER_VAL=""
ROOT_PASS_VAL=""
PMA_STATUS=""

if [ "$INSTALL_MYSQL" == "true" ]; then
    echo ">> Starte Native Installation (LAMP Stack)..." >> "$LOGFILE"

    apt-get install -y mariadb-server mariadb-client apache2 php php-mysql php-mbstring php-zip php-gd php-json php-curl >> "$LOGFILE" 2>&1

    systemctl enable mariadb >> "$LOGFILE" 2>&1; systemctl start mariadb >> "$LOGFILE" 2>&1
    systemctl enable apache2 >> "$LOGFILE" 2>&1; systemctl start apache2 >> "$LOGFILE" 2>&1

    if [ ! -d "/var/www/html/phpmyadmin" ]; then
        wget -q "https://files.phpmyadmin.net/phpMyAdmin/${PMA_VERSION}/phpMyAdmin-${PMA_VERSION}-all-languages.zip" -O pma.zip
        unzip -q pma.zip; rm pma.zip
        mv "phpMyAdmin-${PMA_VERSION}-all-languages" /var/www/html/phpmyadmin
        chown -R www-data:www-data /var/www/html/phpmyadmin
        chmod -R 755 /var/www/html/phpmyadmin

        RANDOM_SECRET=$(openssl rand -base64 24 | tr -dc 'a-zA-Z0-9')
        cp /var/www/html/phpmyadmin/config.sample.inc.php /var/www/html/phpmyadmin/config.inc.php
        sed -i "s/\$cfg\['blowfish_secret'\] = '';/\$cfg\['blowfish_secret'\] = '$RANDOM_SECRET';/" /var/www/html/phpmyadmin/config.inc.php
    fi
    PMA_STATUS="AUTO"

    echo ">> Konfiguriere Datenbank User..." >> "$LOGFILE"

    DB_NAME_VAL="fivem"
    DB_USER_VAL="fivem"
    DB_PASS_VAL=$(openssl rand -base64 12 | tr -dc 'a-zA-Z0-9')
    ROOT_USER_VAL="root"
    ROOT_PASS_VAL=$(openssl rand -base64 12 | tr -dc 'a-zA-Z0-9')

    echo ">> Stoppe MariaDB für Passwort-Reset..." >> "$LOGFILE"

    systemctl stop mariadb >> "$LOGFILE" 2>&1 || systemctl stop mysql >> "$LOGFILE" 2>&1
    sleep 2

    echo ">> Starte mysqld_safe..." >> "$LOGFILE"
    mysqld_safe --skip-grant-tables --skip-networking >/dev/null 2>&1 &

    sleep 10

    echo ">> Setze Root Passwort via Bypass..." >> "$LOGFILE"

    mariadb -e "FLUSH PRIVILEGES;
                ALTER USER 'root'@'localhost' IDENTIFIED VIA mysql_native_password USING PASSWORD('${ROOT_PASS_VAL}');
                FLUSH PRIVILEGES;" >> "$LOGFILE" 2>&1

    echo ">> Beende Safe Mode..." >> "$LOGFILE"
    killall mysqld || killall mariadbd || pkill mysqld
    sleep 5

    echo ">> Starte MariaDB normal neu..." >> "$LOGFILE"
    systemctl start mariadb >> "$LOGFILE" 2>&1 || systemctl start mysql >> "$LOGFILE" 2>&1

    echo ">> Lege restliche User an..." >> "$LOGFILE"

    mysql -u root -p"${ROOT_PASS_VAL}" -e "DROP DATABASE IF EXISTS fivem; DROP USER IF EXISTS '${DB_USER_VAL}'@'%'; DROP USER IF EXISTS '${DB_USER_VAL}'@'localhost'; DROP USER IF EXISTS 'root'@'%'; FLUSH PRIVILEGES;" >> "$LOGFILE" 2>&1

    SQL="CREATE DATABASE IF NOT EXISTS ${DB_NAME_VAL};

         /* FIVEM USER (Nur Remote %) */
         /* Wir löschen localhost explizit, damit % greift */
         CREATE USER '${DB_USER_VAL}'@'%' IDENTIFIED BY '${DB_PASS_VAL}';
         GRANT ALL PRIVILEGES ON ${DB_NAME_VAL}.* TO '${DB_USER_VAL}'@'%';

         /* ROOT USER (Remote %) */
         CREATE USER 'root'@'%' IDENTIFIED BY '${ROOT_PASS_VAL}';
         GRANT ALL PRIVILEGES ON *.* TO 'root'@'%' WITH GRANT OPTION;

         FLUSH PRIVILEGES;"

    if mysql -u root -p"${ROOT_PASS_VAL}" -e "$SQL" >> "$LOGFILE" 2>&1; then
        echo ">> Alle Datenbank-User erfolgreich konfiguriert." >> "$LOGFILE"
    else
        echo ">> FEHLER beim Anlegen der User (Trotz Passwort Reset)." >> "$LOGFILE"
    fi
fi

if [ ! -f "run.sh" ]; then
    echo ">> Lade FiveM Server herunter..." >> "$LOGFILE"
    if ! wget "$ARTIFACT_URL" -O fx.tar.xz >> "$LOGFILE" 2>&1; then
        safe_json_out "false" "Download Error" "DL_ERROR" "" "$LOGFILE"
        exit 1
    fi
    tar xf fx.tar.xz >> "$LOGFILE" 2>&1
    rm fx.tar.xz
fi

echo ">> Richte Autostart ein..." >> "$LOGFILE"
CRON_CMD="@reboot cd $SERVER_ROOT && /usr/bin/screen -dmS fxserver bash -c \"./run.sh\""
(crontab -l 2>/dev/null | grep -v "fxserver"; echo "$CRON_CMD") | crontab -

echo ">> Starte Server..." >> "$LOGFILE"
screen -dmS fxserver bash -c "./run.sh > \"$LOGFILE\" 2>&1"

TIMEOUT=90
COUNT=0
STATUS="UNKNOWN"
PIN=""

while [ $COUNT -lt $TIMEOUT ]; do
    if ! screen -list | grep -q "fxserver"; then
        STATUS="CRASHED"
        if grep -qE "Address already in use|port .* already in use" "$LOGFILE"; then STATUS="PORT_ERROR"; fi
        break
    fi
    if grep -qE "Address already in use|port .* already in use" "$LOGFILE"; then STATUS="PORT_ERROR"; screen -S fxserver -X quit 2>/dev/null; break; fi
    if grep -q "PIN:" "$LOGFILE"; then PIN=$(grep -oP 'PIN: \K[0-9]+' "$LOGFILE" | head -1); STATUS="SUCCESS"; break; fi
    if grep -qP "┃\s+[0-9]{4}\s+┃" "$LOGFILE"; then PIN=$(grep -oP "┃\s+\K[0-9]{4}(?=\s+┃)" "$LOGFILE" | head -1); STATUS="SUCCESS"; break; fi
    sleep 1; ((COUNT++))
done

if [ "$STATUS" == "UNKNOWN" ]; then STATUS="TIMEOUT"; screen -S fxserver -X quit 2>/dev/null; fi

if [ "$STATUS" == "SUCCESS" ]; then
    safe_json_out "true" "" "" "$PIN" "$LOGFILE" "localhost" "$DB_NAME_VAL" "$DB_USER_VAL" "$DB_PASS_VAL" "$PMA_STATUS" "$ROOT_USER_VAL" "$ROOT_PASS_VAL"
else
    case $STATUS in
        "PORT_ERROR") MSG="Port 40120 ist belegt."; CODE="PORT_ERROR";;
        "CRASHED")    MSG="Server im Screen abgestürzt."; CODE="CRASHED";;
        "TIMEOUT")    MSG="Timeout: Keine PIN gefunden."; CODE="TIMEOUT";;
        "WEBSERVER_CONFLICT") MSG="Apache2 ist bereits installiert."; CODE="WEBSERVER_CONFLICT";;
        "FULL_SYSTEM_CONFLICT") MSG="Server und Apache2 gefunden."; CODE="FULL_SYSTEM_CONFLICT";;
        *)            MSG="Unbekannter Fehler."; CODE="UNKNOWN";;
    esac
    safe_json_out "false" "$MSG" "$CODE" "" "$LOGFILE"
fi