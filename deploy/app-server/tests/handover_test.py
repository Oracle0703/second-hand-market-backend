"""Linux/Docker integration test: old service handover, rollback, and retry.

Uses only temporary directories, unique Compose projects, and unused loopback
ports. No application database, upload directory, or external network is used.
"""
import json
import os
from pathlib import Path
import socket
import subprocess
import tarfile
import tempfile
import unittest
import uuid

SCRIPT = Path(__file__).resolve().parents[1] / 'deploy.sh'
IMAGE = os.environ.get('CD_TEST_IMAGE', 'nginx:1.27-alpine')


def run(*args, env=None, check=True):
    return subprocess.run(args, env=env, check=check, text=True,
                          stdout=subprocess.PIPE, stderr=subprocess.STDOUT)


class HandoverTest(unittest.TestCase):
    def test_cutover_recovery_and_retry(self):
        with tempfile.TemporaryDirectory(prefix='cd-handover-') as temporary:
            root = Path(temporary)
            root.chmod(0o755)
            token = uuid.uuid4().hex[:10]
            legacy_project = f'cd-test-old-{token}'
            project = f'cd-test-new-{token}'
            self.ports = []
            for _ in range(2):
                with socket.socket() as sock:
                    sock.bind(('127.0.0.1', 0))
                    self.ports.append(sock.getsockname()[1])
            old = root / 'legacy'
            new = root / 'new'
            (old / 'site').mkdir(parents=True)
            (new / 'deploy').mkdir(parents=True)
            (new / 'frontend' / 'releases').mkdir(parents=True)
            (new / 'incoming').mkdir()
            (old / '.env').write_text(f'COMPOSE_PROJECT_NAME={legacy_project}\n')
            (old / 'site' / 'index.html').write_text('legacy')
            (old / 'site' / 'healthz').write_text('ok')
            legacy_compose = old / 'docker-compose.yml'
            compose = new / 'deploy' / 'docker-compose.yml'
            health = {'test': ['CMD', 'wget', '-qO-', 'http://127.0.0.1/healthz'],
                      'interval': '1s', 'timeout': '1s', 'retries': 2}
            legacy_services = {}
            for name, port in zip(('api', 'web'), self.ports):
                legacy_services[name] = {
                    'image': IMAGE, 'ports': [f'127.0.0.1:{port}:80'],
                    'volumes': [f'{old}/site:/usr/share/nginx/html:ro'],
                    'healthcheck': health}
            # A sentinel dependency must survive handover without being recreated.
            legacy_services['database'] = {'image': IMAGE}
            legacy_compose.write_text(json.dumps({'services': legacy_services}))
            compose.write_text(json.dumps({'name': '${COMPOSE_PROJECT_NAME}', 'services': {
                'api': {'image': '${BACKEND_IMAGE}',
                        'ports': [f'127.0.0.1:{self.ports[0]}:80'],
                        'volumes': [f'{old}/site:/usr/share/nginx/html:ro'],
                        'healthcheck': health},
                'web': {'image': IMAGE,
                        'ports': [f'127.0.0.1:{self.ports[1]}:80'],
                        'volumes': [f'{new}/frontend/current:/usr/share/nginx/html:ro'],
                        'healthcheck': dict(health, test=['CMD-SHELL',
                            '! grep -q broken /usr/share/nginx/html/index.html && '
                            'wget -qO- http://127.0.0.1/healthz'])}}}))
            clean_env = {k: v for k, v in os.environ.items() if not k.startswith('COMPOSE_')}
            legacy = ['docker', 'compose', '--project-directory', str(old),
                      '--env-file', str(old / '.env'), '-f', str(legacy_compose)]
            env = dict(clean_env, DEPLOY_ROOT=str(new), BACKEND_IMAGE=IMAGE,
                       COMPOSE_PROJECT_NAME=project, API_HOST_PORT=str(self.ports[0]),
                       WEB_HOST_PORT=str(self.ports[1]), LEGACY_COMPOSE_FILE=str(legacy_compose))
            current = ['docker', 'compose', '-f', str(compose)]

            def ids(command, environment):
                return {service: run(*command, 'ps', '--status', 'running', '-q', service,
                                     env=environment).stdout.strip()
                        for service in ('api', 'web', 'database')
                        if service != 'database' or command is legacy}

            def deploy(release, content, succeeds):
                site = root / ('site-' + release)
                site.mkdir()
                (site / 'index.html').write_text(content)
                (site / 'healthz').write_text('ok')
                archive = new / 'incoming' / (release + '.tar.gz')
                with tarfile.open(archive, 'w:gz') as bundle:
                    bundle.add(site, arcname='.')
                result = run('bash', str(SCRIPT), env=dict(env, RELEASE_ID=release,
                             FRONTEND_ARCHIVE=str(archive)), check=False)
                self.assertEqual(result.returncode == 0, succeeds, result.stdout)
                if not succeeds:
                    self.assertIn('Previous services restored', result.stdout)
                    self.assertNotIn('Recovery failed', result.stdout)
                return result

            try:
                run(*legacy, 'up', '-d', '--wait', env=clean_env)
                before = ids(legacy, clean_env)
                deploy('a' * 40, 'broken', False)
                self.assertEqual(ids(legacy, clean_env), before)
                self.assertFalse((new / 'frontend' / 'current').exists())
                self.assertFalse(any(ids(current, env).values()))
                print('PASS: failed first cutover restores original containers and database', flush=True)

                # Retry after failed containers exist must still find the legacy project.
                deploy('b' * 40, 'healthy-release', True)
                self.assertTrue(all(ids(current, env).values()))
                legacy_after = ids(legacy, clean_env)
                self.assertEqual(legacy_after, {'api': '', 'web': '', 'database': before['database']})
                print('PASS: retry hands over ports without modifying the database', flush=True)

                previous_api = ids(current, env)['api']
                previous_image = run('docker', 'inspect', '--format', '{{.Image}}', previous_api).stdout.strip()
                deploy('c' * 40, 'broken', False)
                self.assertEqual((new / 'frontend' / 'current' / 'index.html').read_text(), 'healthy-release')
                restored = ids(current, env)
                self.assertTrue(all(restored.values()))
                self.assertEqual(run('docker', 'inspect', '--format', '{{.Image}}', restored['api']).stdout.strip(), previous_image)
                self.assertEqual(ids(legacy, clean_env), legacy_after)
                print('PASS: managed release failure restores API image and frontend', flush=True)

                # A repeated deployment needs neither legacy services nor a new archive.
                repeat = run('bash', str(SCRIPT), env=dict(env, RELEASE_ID='b' * 40,
                             FRONTEND_ARCHIVE=str(new / 'incoming' / ('b' * 40 + '.tar.gz'))))
                self.assertIn('Deployed release', repeat.stdout)
                self.assertTrue(all(ids(current, env).values()))
                print('PASS: deploying the same release again succeeds', flush=True)
            finally:
                run(*current, 'down', '--remove-orphans', env=env, check=False)
                run(*legacy, 'down', '--remove-orphans', env=clean_env, check=False)


if __name__ == '__main__':
    unittest.main()
