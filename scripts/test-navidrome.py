#!/usr/bin/env python3
"""Disposable Docker integration test. Requires Go, ffmpeg and zensync:migration-test.
Creates synthetic audio; never accesses a user's Navidrome server or Deezer account.
"""
import hashlib
import http.cookiejar
import json
import os
import pathlib
import secrets
import subprocess
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request


def run(*args):
    return subprocess.check_output(args, text=True).strip()


def wait(check, seconds=45):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        try:
            value = check()
            if value:
                return value
        except (OSError, KeyError):
            pass
        time.sleep(0.4)
    raise AssertionError('Condition timed out')


def request_json(url, body=None, headers=None, client=None):
    req = urllib.request.Request(url, data=body, headers=headers or {})
    with (client or urllib.request).urlopen(req) if client is None else client.open(req) as response:
        return json.load(response)


def main():
    suffix = secrets.token_hex(4)
    network = 'zensync-proof-' + suffix
    navname, appname = network + '-nav', network + '-app'
    names = []
    root = pathlib.Path(__file__).resolve().parents[1]
    password = secrets.token_hex(20)
    run('docker', 'network', 'create', network)
    try:
        with tempfile.TemporaryDirectory(prefix='zensync-proof-') as temp:
            music = pathlib.Path(temp, 'music')
            music.mkdir()
            for title, filename in [('First song', 'first'), ('Second song', 'second')]:
                run('ffmpeg', '-v', 'error', '-f', 'lavfi', '-i', 'sine=frequency=440:duration=2', '-metadata', 'title='+title, '-metadata', 'artist=Test artist', '-metadata', 'album=Test album', '-metadata', 'SOURCEID='+('1' if filename=='first' else '2'), str(music / (filename+'.flac')))
            run('docker', 'run', '-d', '--name', navname, '--network', network, '-p', '127.0.0.1::4533', '-v', str(music)+':/music:ro', '-e', 'ND_SCANSCHEDULE=0', '-e', 'ND_SUBSONIC_DEFAULTREPORTREALPATH=true', 'deluan/navidrome:0.64.2')
            names.append(navname)
            navport = json.loads(run('docker', 'inspect', navname))[0]['NetworkSettings']['Ports']['4533/tcp'][0]['HostPort']
            navurl = 'http://127.0.0.1:'+navport
            wait(lambda: urllib.request.urlopen(navurl+'/ping').status == 200)
            request_json(navurl+'/auth/createAdmin', json.dumps({'username':'navadmin','password':password}).encode(), {'Content-Type':'application/json'})
            def nav(endpoint, **values):
                salt=secrets.token_hex(8)
                values.update(u='navadmin', t=hashlib.md5((password+salt).encode()).hexdigest(), s=salt, v='1.16.1', c='ZenSyncTest', f='json')
                data=request_json(navurl+'/rest/'+endpoint+'.view',urllib.parse.urlencode(values,doseq=True).encode(),{'Content-Type':'application/x-www-form-urlencoded'})['subsonic-response']
                assert data['status']=='ok', data
                return data
            nav('startScan')
            tracks=wait(lambda: (lambda x:x if len(x)==2 else None)(nav('search3',query='',songCount=100)['searchResult3'].get('song',[])))
            testenv=dict(os.environ,ZENSYNC_TEST_NAV_URL=navurl,ZENSYNC_TEST_NAV_PASSWORD=password,ZENSYNC_TEST_MUSIC=str(music))
            subprocess.check_call([os.environ.get('ZENSYNC_GO','go'),'test','./internal/app','-run','TestRealNavidromeAssetVerification|TestRealLegacyRelocation|TestRealNavidromeArtistMerge|TestRealNavidromeScreenshotReview|TestRealNavidromeSeparateKnownSkateArtists','-count=1'],cwd=root,env=testenv)
            run('docker','run','-d','--name',appname,'--network',network,'-p','127.0.0.1::8080','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--tmpfs','/tmp','-v','/data','-v',str(music)+':/music','zensync:migration-test')
            names.append(appname)
            appport=json.loads(run('docker','inspect',appname))[0]['NetworkSettings']['Ports']['8080/tcp'][0]['HostPort']
            base='http://127.0.0.1:'+appport
            wait(lambda:request_json(base+'/readyz')['status']=='ready')
            client=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
            csrf=''
            def api(path, body=None, raw=None):
                headers={'Origin':base,'Content-Type':'application/json','X-CSRF-Token':csrf}
                data=raw if raw is not None else None if body is None else json.dumps(body).encode()
                return request_json(base+'/api/v2'+path,data,headers,client)
            assert api('/setup')['required']
            api('/setup',{'username':'admin','password':password})
            csrf=api('/session',{'username':'admin','password':password})['csrf']
            def put(path,body):
                request=urllib.request.Request(base+'/api/v2'+path,data=json.dumps(body).encode(),method='PUT',headers={'Origin':base,'X-CSRF-Token':csrf,'Content-Type':'application/json'})
                return json.load(client.open(request))
            put('/connections',{'kind':'shared','url':'http://'+navname+':4533','username':'navadmin','password':password})
            put('/connections',{'kind':'personal','username':'navadmin','password':password})
            mid=api('/imports',raw=(root/'internal/apple/testdata/library.xml').read_bytes())['id']
            detail=api('/imports/'+mid)
            playlist=next(p for p in detail['playlists'] if p['name']=='Roadtrip')
            api('/imports/'+mid+'/commit',{'playlists':[playlist['id']],'favorites':True})
            for song in detail['songs']:
                match=next((s for s in tracks if s['title']==song['metadata']['title']),None)
                if match:
                    api('/imports/'+mid+'/songs/'+song['id']+'/resolve',{'serverId':match['id']})
            api('/imports/'+mid+'/publish',{})
            def published():
                d=api('/imports/'+mid)
                p=next(p for p in d['playlists'] if p['id']==playlist['id'])
                if p['state']=='paused' or d['migration']['error']:
                    raise AssertionError(json.dumps(d['migration'])+' '+json.dumps(p))
                return p if p['remoteId'] and p['state']=='active' else None
            p=wait(published)
            remote=nav('getPlaylist',id=p['remoteId'])['playlist']
            assert [s['title'] for s in remote['entry']]==['First song','Second song','First song'],remote
            first=next(t for t in tracks if t['title']=='First song')
            assert nav('getSong',id=first['id'])['song'].get('starred')
            nav('unstar',id=first['id'])
            nav('updatePlaylist',playlistId=p['remoteId'],name='Edited outside ZenSync')
            api('/imports/'+mid+'/publish',{})
            wait(lambda:next(p for p in api('/imports/'+mid)['playlists'] if p['id']==playlist['id'])['state']=='paused')
            assert nav('getPlaylist',id=p['remoteId'])['playlist']['name']=='Edited outside ZenSync'
            assert not nav('getSong',id=first['id'])['song'].get('starred')
            api('/imports/'+mid+'/handover',{})
            assert api('/imports/'+mid)['migration']['state']=='handed_over'
            run('docker','restart',appname)
            appport=json.loads(run('docker','inspect',appname))[0]['NetworkSettings']['Ports']['8080/tcp'][0]['HostPort']
            base='http://127.0.0.1:'+appport
            wait(lambda:request_json(base+'/readyz')['status']=='ready')
            assert not api('/setup')['required']
            assert api('/imports/'+mid)['migration']['state']=='handed_over'
            assert len(nav('getPlaylists')['playlists']['playlist'])==1
            run('docker','exec',appname,'/app/zensync','healthcheck')
            print('PASS: non-root Docker, first-access setup, real Navidrome 0.64.2 connection, XML commit, ordered duplicate playlist entries, favorites, legacy relocation with stable references, external-edit pause, handover and restart persistence.')
    finally:
        for name in reversed(names):
            subprocess.run(['docker','rm','-fv',name],stdout=subprocess.DEVNULL,check=False)
        subprocess.run(['docker','network','rm',network],stdout=subprocess.DEVNULL,check=False)

if __name__=='__main__':
    main()
