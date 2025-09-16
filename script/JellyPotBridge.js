// ==UserScript==
// @name         JellyPotBridge
// @namespace    http://tampermonkey.net/
// @version      1.0.0
// @description  JellyPotBridge
// @license      MIT
// @author       @Hattiss
// @include      */web/
// ==/UserScript==

(function () {
    'use strict';
    setInterval(function () {
        let potplayer = document.querySelectorAll("div#itemDetailPage:not(.hide) #jellyPot")[0];
        if (!potplayer) {
            let mainDetailButtons = document.querySelectorAll("div#itemDetailPage:not(.hide) .mainDetailButtons .detailButton[data-action='resume']")[0];
            if (mainDetailButtons) {
                let buttonhtml = `
                  <button id="jellyPot" type="button" class="button-flat btnPlay detailButton emby-button" title="Potplayer"><div class="detailButton-content"><span class="material-icons detailButton-icon icon-PotPlayer"></span></div></button>
                  `
                mainDetailButtons.insertAdjacentHTML('beforebegin', buttonhtml)
                document.querySelector("div#itemDetailPage:not(.hide) #jellyPot").onclick = callJellyPot;
                //add icons
                document.querySelector("div#itemDetailPage:not(.hide) .icon-PotPlayer").style.cssText += 'background: url(https://cdn.jsdelivr.net/gh/bpking1/embyExternalUrl@0.0.2/embyWebAddExternalUrl/icons/icon-PotPlayer.webp)no-repeat;background-size: 100% 100%';
            }
        }
    }, 1000);

    async function getItemId() {
        let userId = ApiClient._serverInfo.UserId;
        let itemId = /\?id=(\w*)/.exec(window.location.hash)[1];
        let response = await ApiClient.getItem(userId, itemId);
        console.log(response);
        //获取当前剧集的下一集
        if (response.Type === "Series") {
            let seriesNextUpItems = await ApiClient.getNextUpEpisodes({SeriesId: itemId, UserId: userId});
            return seriesNextUpItems.Items[0].Id;
        }
        //获取当前季的第一集
        if (response.Type === "Season" || response.Type === "BoxSet") {
            let seasonItems = await ApiClient.getItems(userId, {parentId: itemId});
            return seasonItems.Items[0].Id;
        }
        return itemId;
    }

    async function callJellyPot() {
        let itemId = await getItemId();
        let poturl = `jellypot://${itemId}`;
        const iframe = document.createElement('iframe');
        iframe.style.display = 'none';
        iframe.src = poturl;
        console.log(poturl)
        document.body.appendChild(iframe);
        setTimeout(() => {
            document.body.removeChild(iframe);
        }, 100);
    }
})();