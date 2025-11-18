go install github.com/riverqueue/river/cmd/river@latest
npm install pm2 -g
npm install --save-dev dbmate -g
sudo ln -s "$(which river)" /usr/local/bin/river
udo ln -s "$(which pm2)" /usr/local/bin/pm2