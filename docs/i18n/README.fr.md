# launchbay

> Tous les favoris que vous avez oubliés, sur une seule page — auto-hébergé, hors ligne, sans inscription.

[![License: MIT](https://img.shields.io/badge/License-MIT-1f6feb)](../../LICENSE) [![365 Open Source Plan #042](https://img.shields.io/badge/365%20Open%20Source%20Plan-%23042-1f6feb)](https://github.com/rockbenben/365opensource) [![ci](https://img.shields.io/github/actions/workflow/status/rockbenben/launchbay/ci.yml?label=ci&branch=main)](https://github.com/rockbenben/launchbay/actions/workflows/ci.yml)

[▶ Démarrage rapide](#démarrage-rapide) · [⬇ Releases](https://github.com/rockbenben/launchbay/releases)

[English](../../README.md) · [简体中文](../../README.zh.md) · [繁體中文](README.zh-Hant.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · [Español](README.es.md) · [Português](README.pt.md) · **Français** · [Deutsch](README.de.md) · [Italiano](README.it.md) · [Русский](README.ru.md) · [العربية](README.ar.md) · [हिन्दी](README.hi.md) · [বাংলা](README.bn.md) · [ไทย](README.th.md) · [Türkçe](README.tr.md) · [Tiếng Việt](README.vi.md) · [Bahasa Indonesia](README.id.md)

![Page de profil launchbay : recherche dans la barre supérieure, groupes en barre latérale et tuiles](../../docs/home-1280.en.webp)

Rassemblez sur une seule page toutes les adresses que vous n'arrivez jamais à retenir : l'administration du NAS, le routeur, l'intranet de l'entreprise, le tableau de bord de surveillance. Faites-en la page d'accueil de votre navigateur — **un clic, aucun temps d'attente.**

L'application tourne sur votre propre machine — un NAS domestique, un Raspberry Pi, un routeur ou un serveur sur le réseau local de l'entreprise — de sorte que vos favoris ne touchent jamais les serveurs de quelqu'un d'autre.

## Pourquoi launchbay

La plupart des outils de gestion de favoris se distinguent sur quatre points : à qui appartient le serveur hébergeant les données, comment un foyer les partage, si cela fonctionne hors ligne et si l'on peut organiser ses liens par glisser-déposer.

| Critère de comparaison | Favoris intégrés du navigateur | Services de favoris dans le cloud | launchbay |
|---|---|---|---|
| Où sont les données | Dans votre profil de navigateur, lié à votre compte | Sur les serveurs du fournisseur | **Votre propre machine** |
| Une session par personne | Via les profils du navigateur | Par compte | **Chacun le sien, séparés sur le serveur** |
| Hors ligne | Uniquement en local | Ne fonctionne pas | **Cache + file d'attente hors ligne** |
| Organisation par glisser-déposer | Arborescence de dossiers, pas une mise en page | La plupart non | **Déposer pour enregistrer, glisser pour réordonner** |
| Qui d'autre peut lire | Votre compte de navigateur | Le fournisseur | **Personne via l'application** |

Aucun compte, aucun script tiers, aucune télémétrie. Les seules requêtes externes servent à récupérer la page que vous ajoutez (pour son titre) et son favicon ; la récupération d'un favicon échoué est suspendue pendant 24 heures.

**Si vous êtes seul à l'utiliser, les favoris intégrés de votre navigateur suffisent probablement.** launchbay apporte trois atouts décisifs :

- **Ouverture instantanée.** La page est rendue côté serveur : les favoris et leurs icônes sont présents dans le HTML avant même qu'un script ne s'exécute. L'objectif est un LCP premier affichage en réseau local < 300 ms ; un démarrage à froid sur un NAS réel n'a pas encore été mesuré (voir [Limites](#limites)).
- **Chacun son espace, invisible pour les autres.** Chaque profil peut avoir son propre code PIN de 4 à 6 chiffres. L'administrateur peut créer, supprimer et réinitialiser les codes PIN, mais tous les points de terminaison de contenu refusent la clé d'administration avec une erreur 403 — il n'y a **aucune route** pour lire vos favoris. Il ne s'agit pas d'une protection cryptographique contre l'administrateur ou le réseau local (n'importe qui sur le LAN peut modifier un cookie), mais de la garantie que le serveur sépare strictement les profils.
- **Toute fausse manipulation est réversible.** Chaque modification par lot enregistre d'abord un instantané. L'option « Revenir » indique combien de favoris et quels groupes contenait chaque version antérieure, et conserve la version actuelle avant restauration — ainsi, un retour arrière effectué par erreur peut lui-même être annulé. L'historique est conservé et n'est jamais effacé : chaque ligne indique qui a modifié ce profil, quand, par quel moyen (page web, IA, extension, CLI) et quelle entrée — sans jamais dévoiler le contenu.

## Ce qu'il couvre

| Aspect | Si cela correspond à vos besoins |
|---|---|
| Comment l'exécuter | `docker compose up -d`, ou le binaire statique (amd64 / arm64) |
| Où il tourne | Un NAS personnel, Raspberry Pi, routeur ou une machine du réseau interne de l'entreprise |
| Qui l'utilise | Une personne seule, une famille ou une petite équipe — un profil chacun ; l'administrateur gère les comptes, pas le contenu |
| Navigateurs | Tout navigateur moderne ; même sans JavaScript, chaque favori reste visible et cliquable |
| Ce qu'il faut installer | Seulement Docker : aucune bibliothèque tierce, aucune base de données, pas de Node au moment du build |
| Où sont les données | Un répertoire ordinaire, `data/` ; faire une sauvegarde consiste simplement à le copier |
| Langue de l'interface | 4 langues d'interface (chinois simplifié, chinois traditionnel, anglais, arabe RTL) ; choisissez la valeur par défaut lors de la configuration, puis chaque profil choisit la sienne |

## Ce qu'il fait

**Enregistrer et organiser**

- **Déposez, c'est enregistré** : faites glisser un lien depuis la barre de favoris, la barre d'adresse ou un autre onglet directement sur un groupe. Aucun dialogue — le dépôt suffit.
- **Glisser pour réordonner** : réorganisez au sein d'un groupe, déplacez entre les groupes ou déposez sur le titre d'un groupe pour l'ajouter à la suite. Un appui long ou un raccourci clavier déclenche la sélection multiple pour étiqueter, déplacer ou supprimer en un seul geste.
- **Enregistrer depuis n'importe quelle page** : faites glisser le bookmarklet vers votre barre de favoris ; un clic récupère l'adresse, un second appui l'enregistre.
- **Importer depuis votre navigateur** : importez le fichier de favoris exporté depuis Chrome / Edge / Firefox et consultez un rapport détaillé entrée par entrée — ce qui a été importé, ce qui a été ignoré et pourquoi. La page web peut importer directement ou afficher d'abord le rapport ; la CLI applique l'import dans la même commande, à moins de passer `--dry-run`.
- **Emporter un profil complet** : un seul fichier regroupe les favoris **et** l'apparence de la page (thème, disposition, affichage, deuxième ligne, nom de page, langue, mode d'adresse). Le code PIN, l'identité du profil et l'image d'arrière-plan en sont délibérément exclus.

**Trouver**

- **Groupes et recherche** : la barre supérieure recherche par nom, adresse, note et étiquette ; appuyez sur Entrée et le serveur répond — aucun JavaScript n'est nécessaire. Lorsque les scripts sont activés, appuyez sur `/` ou ouvrez la palette `⌘/Ctrl K`.
- **Groupes affichés plus grands** : mettez en avant les groupes que vous utilisez le plus souvent dans un bandeau de grandes cartes en haut de page.
- **Plus de 10 000 icônes, hors ligne** : les icônes Tabler et Simple Icons sont compressées au format gzip dans le binaire ; le favicon d'un site n'est récupéré qu'une seule fois puis servi depuis le cache local.

**À la maison et à distance**

- **Deux adresses par favori** : définissez une adresse principale et une adresse WAN optionnelle. Le mode « Auto » s'adapte à l'adresse avec laquelle vous avez ouvert launchbay ; Paramètres → Adresses de favoris permet de forcer ce choix pour le navigateur en cours. La recherche et la copie utilisent l'adresse active, et les favoris sans adresse WAN conservent l'adresse principale. Derrière un reverse proxy, conservez l'en-tête `Host` d'origine.

**La page elle-même**

- **Un panneau de paramètres unique** : barre supérieure → Paramètres. Il rassemble l'apparence et la langue, la liste des groupes, la sauvegarde et l'historique, ainsi que **Mon code PIN** — pour verrouiller ou déverrouiller votre profil sans dépendre de l'administrateur.
- **Fonctionne hors ligne** : la dernière page consultée reste en cache, et les favoris ajoutés hors ligne — ou lorsque la machine est éteinte — sont mis en file d'attente puis rejoués dès que le navigateur se reconnecte ; cliquez sur **Envoyer maintenant** si le serveur était temporairement indisponible.
- **Clés dédiées pour les scripts et l'IA** : une API REST avec des jetons restreints au profil pour l'extension, la CLI et l'automatisation. Un jeton tentant d'accéder à un autre profil reçoit une erreur 403.

## Démarrage rapide

```bash
git clone https://github.com/rockbenben/launchbay
cd launchbay
docker compose up -d
```

Aucun fichier `.env`, aucun environnement Go à installer — l'image Docker est simplement téléchargée.

Ouvrez **http://<adresse LAN de cette machine>:7600** depuis n'importe quel appareil connecté au réseau local — **pas `localhost`**, qui désigne « cet appareil » et ouvrirait le smartphone sur lequel vous tapez l'adresse. Si vous êtes directement sur la machine hôte, `http://localhost:7600` fonctionne.

L'image est étiquetée `:latest` ; la première commande `up` télécharge la version la plus récente. Une machine déjà en fonctionnement conserve son image locale : lancez d'abord `docker compose pull` pour mettre à jour.

> **Changez impérativement la clé d'administration par défaut.** Le fichier compose fournit `launchbay123`, qui est publique. Toute personne sur votre réseau local connaissant cette clé peut créer ou supprimer des profils et réinitialiser les codes PIN (sans pouvoir lire les favoris). Définissez votre propre valeur dans `LAUNCHBAY_ADMIN_TOKEN` puis relancez `docker compose up -d` :
>
> ```yaml
> LAUNCHBAY_ADMIN_TOKEN: "votre-propre-cle-admin"
> ```

Le premier accès affiche la page de configuration initiale : nommez le premier profil et définissez un code PIN de 4 à 6 chiffres. Il n'existe aucun identifiant par défaut, et la page de configuration se verrouille définitivement dès qu'un profil est créé.

**« Pas de code PIN » est une case à cocher explicite, pas un champ laissé vide.** Laisser le champ du code PIN vide est refusé ; cocher **Pas de code PIN** crée un profil accessible à tous sur le réseau local. Ce choix est délibéré : l'erreur coûteuse consisterait à taper trois chiffres et appuyer par inadvertance sur Entrée, publiant silencieusement les favoris personnels sur tout le réseau de la maison. Un profil sans code PIN n'affiche pas de champ PIN sur la page de sélection. Vous pourrez ajouter ou retirer ce verrou ultérieurement dans **Mon code PIN** ; le retrait n'exige l'ancien code que si un code est actuellement défini.

Le premier profil n'est **pas un compte administrateur**. La machine s'administre via `LAUNCHBAY_ADMIN_TOKEN` : rendez-vous sur `/admin`, saisissez la clé une fois et vous obtiendrez un cookie réservé à l'identité administrateur (valable 30 jours).

**Les utilisateurs suivants n'ont pas besoin d'attendre l'administrateur.** La page de sélection comporte une section repliée **« Un nouveau pour vous »** : ouvrez-la, renseignez un nom et un code PIN, et le profil est immédiatement créé (`POST /new` ; la route API correspondante est `POST /api/v1/profiles/new`). Aucune authentification n'est exigée, permettant à tout équipement du réseau local d'en créer un ; la carte apparaît aussitôt et l'administrateur peut la supprimer à tout moment. Cela crée un profil **vide** protégé par son propre PIN, qui ne confère aucun droit sur les favoris des autres. Les noms peuvent contenir jusqu'à 60 caractères ; réutiliser un nom existant rendra les deux cartes visuellement indifférenciables.

**La suppression d'un profil est la seule opération irréversible**, c'est pourquoi elle réclame à nouveau la clé de la machine : la page de confirmation indique précisément le nombre de favoris et d'instantanés qui seront supprimés, et vous devez ressaisir `LAUNCHBAY_ADMIN_TOKEN`. La route API (`DELETE /api/v1/profiles`) n'accepte que cette clé brute ; un simple cookie administrateur renvoie une erreur 403. Les instantanés et fonds d'écran personnalisés disparaissent avec le profil, de sorte qu'aucune restauration ne pourra le récupérer.

Cette même page propose un menu déroulant **Langue par défaut** ; choisissez-la lors de la configuration. Elle définit la **valeur par défaut pour toute la machine** ; chaque utilisateur peut ensuite choisir sa propre langue dans son panneau, et les modifications apportées par l'administrateur ne toucheront que les personnes n'ayant pas fait de choix explicite.

### Clé d'administration et jetons

| Variable | Obligatoire ? | Périmètre couvert |
|---|---|---|
| `LAUNCHBAY_ADMIN_TOKEN` | **Obligatoire**, une clé simple | Gestion des comptes uniquement : création/suppression de profils, définition de PIN, activation/désactivation, état et historique global. Tous les points de terminaison de contenu (`/api/v1/data`, `/batch`, `/export`, …) renvoient 403 — impossible de lire les favoris |
| `LAUNCHBAY_WRITE_TOKENS` | Facultative, vide par défaut | Jetons d'écriture restreints au profil, au format `name:secret:profile` (nom:secret:profil). Seuls l'extension pour navigateur, la CLI (`revert` / `import-bookmarks` / `backup` / `restore` / `profile fix-names`) ainsi que l'IA ou les scripts en ont besoin |

> **Pourquoi est-elle vide par défaut ?** La page des favoris, l'administration, la restauration et l'importation reposent tous sur le cookie de session ; aucun jeton n'est requis. Les trois usages nécessitant un jeton sont installés de manière volontaire. Fournir une clé par défaut reviendrait à doter chaque installation d'un sésame universel capable de modifier le profil `dad` (`dad` étant l'identifiant le plus courant).

Un profil ne possède pas d'attribut « est administrateur ». La page d'administration présente les actions de gestion de compte ainsi que, pour chaque profil, le nombre d'éléments, la version, la date de dernière modification et l'indication éventuelle qu'il n'a **pas de code PIN**.

> **L'identifiant du profil est dérivé de son nom et ne s'appelle pas nécessairement `dad`.** Les lettres et chiffres ASCII sont convertis en slug (`Dad` → `dad`, `mon nas` → `mon-nas`) ; les caractères sans équivalent ASCII exploitable basculent sur la séquence `home` / `home-2`. Un jeton restreint à `…:dad` ne correspondra alors à aucun profil et recevra une 403 — adaptez le périmètre du jeton à l'identifiant réel (recommandé) ou donnez un nom en caractères ASCII au premier profil.

Le port hôte par défaut est 7600. Sur certaines machines Windows, Hyper-V réserve une plage de ports (7532–7631 sur la machine de test) ; si la commande `up` échoue, choisissez un port libre en dehors de cette plage : `LAUNCHBAY_HOST_PORT=8901 docker compose up -d` (le port interne au conteneur reste inchangé). Les noms de projet et de conteneur étant fixés à `launchbay`, les commandes `docker logs launchbay` et `docker exec launchbay …` fonctionnent quel que soit le dossier où le dépôt est cloné.

### Extension de navigateur (facultative)

Le dossier `extension/` contient une extension pour Chrome / Edge (Manifest V3) : faites un clic droit sur la page en cours pour l'enregistrer dans un profil précis, ou sauvegardez tous les onglets de la fenêtre d'un seul coup. Elle ne fait pas partie de l'image Docker ; installez-la vous-même.

Elle nécessite un jeton restreint au profil visé, car elle ne peut pas lire le cookie de session. Renseignez `LAUNCHBAY_WRITE_TOKENS`, redémarrez le conteneur, puis :

1. Ouvrez `chrome://extensions` et activez le **Mode développeur**
2. Cliquez sur **Charger l'extension non empaquetée** et sélectionnez le dossier `extension/`
3. Ouvrez les options de l'extension et renseignez l'adresse de la machine (sans barre oblique finale), l'identifiant du profil par défaut et le secret du jeton
4. Cliquez sur **Tester cette machine**

Une réponse 403 lors du test n'est pas une panne réseau : elle indique simplement que le jeton et l'identifiant de profil ne correspondent pas. Corrigez le périmètre dans `LAUNCHBAY_WRITE_TOKENS`, redémarrez et renouvelez le test.

Une fois installée : clic droit sur n'importe quelle page → enregistrer. La page des options permet également d'importer l'intégralité de l'arborescence des favoris du navigateur (l'autorisation d'accès aux favoris n'est demandée qu'à la première utilisation). Quel que soit le moyen utilisé (web, extension, CLI), l'importation ne peut s'effectuer que dans un profil **vide** — elle remplace les favoris du profil au lieu de s'y ajouter. Chrome affiche un avertissement « Lire et modifier vos données sur tous les sites » car l'extension a besoin des permissions `http://*/*` et `https://*/*` pour intercepter le clic droit ; le jeton reste stocké dans `chrome.storage.local` et n'est jamais synchronisé. Le support de Firefox n'a pas été testé.

### Permissions du répertoire de données

Les données sont écrites dans `./data` à côté du fichier `docker-compose.yml`, et correspondent toujours à `/data` à l'intérieur du conteneur. Pour les déplacer, modifiez la ligne `volumes` :

```yaml
    volumes:
      - /volume1/docker/launchbay:/data
```

Sous Windows, un chemin absolu tel que `D:` entre en conflit avec le séparateur ; utilisez la syntaxe détaillée :

```yaml
    volumes:
      - type: bind
        source: D:/launchbay-data
        target: /data
```

Le conteneur s'exécute avec l'**UID 65534**, alors que le répertoire sur la machine hôte appartient souvent à root — la création de profils, les imports et les écritures échouent alors. Attribuez les droits à cet utilisateur sur l'hôte :

```bash
sudo chown -R 65534:65534 ./data
```

N'essayez pas de contourner ce problème avec `--user root` : cela autoriserait le service à modifier n'importe quel fichier de votre machine en tant que root. Le contrôle d'état (healthcheck) crée et supprime un fichier test à chaque vérification ; un problème de droits apparaît donc sous la forme d'un état **unhealthy** dans `docker ps`, et le journal affiche la commande chown appropriée. La page de configuration initiale indique la même consigne (`UID 65534` dans un conteneur ; votre propre utilisateur si vous lancez directement le binaire).

## Utilisation

| Ce que vous souhaitez faire | Emplacement |
|---|---|
| Modifier l'apparence, l'affichage, les groupes, l'arrière-plan, le mode d'adresse ou la façon dont s'ouvrent les favoris | Paramètres dans la barre supérieure, ou `POST /prefs`, `/prefs/background`, `/prefs/group`, `/prefs/tabs` |
| Modifier votre propre code PIN | Mon code PIN dans votre panneau de paramètres, ou `POST /prefs/pin` / `POST /api/v1/profiles/self/pin` |
| Créer ou supprimer un profil, définir le PIN d'un utilisateur, désactiver un compte | `/admin` (ou le lien en bas de la page de sélection). La suppression demande à nouveau la clé ; côté CLI, la gestion des profils se limite à `profile create` et `profile rebuild` |
| Créer un profil supplémentaire pour vous-même | La section repliée « Un nouveau pour vous » sur la page de sélection, ou `POST /api/v1/profiles/new` (sans authentification) |
| Nettoyer les noms qui sont en réalité de simples adresses URL | `launchbay profile fix-names --profile <id>` (testez d'abord avec `--dry-run`) |
| Laisser une IA ou un script modifier les favoris | `POST /api/v1/batch` avec `expectedRev` |
| Récupérer un profil endommagé | `POST /api/v1/profiles/rebuild`, ou `launchbay profile rebuild` |
| Consulter le détail de chaque modification | `GET /api/v1/history` (actions et chemins uniquement, jamais les contenus) |
| Sauvegarder ou déplacer un profil entier | `GET /api/v1/backup` / `POST /api/v1/restore`, `launchbay backup` / `restore`, ou Sauvegarde et historique dans les Paramètres |

Autres routes API : `POST /api/v1/usage` comptabilise les clics, `POST /api/v1/resolve` convertit une URL en titre et icône, et `GET /api/v1/status` fournit le récapitulatif global pour l'administrateur.

Les sauvegardes excluent le code PIN, l'identité du profil et l'image d'arrière-plan. Une sauvegarde contenant des favoris ne peut être restaurée que dans un profil vide ; une sauvegarde ne contenant que des paramètres peut être importée n'importe où. Les imports vérifient la révision sous le verrou de profil et répondent avec un code 409 `[rev-conflict]` accompagné de `currentRev` en cas de conflit. Les paramètres appliqués de cette manière n'incrémentent pas la révision et n'apparaissent pas dans la liste des versions à restaurer.

**La liste intégrale des points de terminaison, des corps de requête et des codes d'erreur figure dans [AGENTS.md](../../AGENTS.md)** — le contrat officiel destiné aux agents IA, vérifié automatiquement par rapport à la table des routes. Il n'est pas dupliqué ici.

**Conventions de la CLI** : les sorties normales vont sur stdout, les erreurs sur stderr, et chaque échec cite le code retourné par le serveur (`[rev-conflict]`, `[ops-invalid]`). La commande `import-bookmarks` attend le fichier en argument positionnel et applique les modifications au cours de la même exécution que l'affichage du rapport (l'option `--dry-run` s'arrête avant l'écriture) ; les sauvegardes et exports lisent la réponse en flux continu, acceptant sans problème les fichiers de plus de 4 Mio. Les options `--help` et `-h` quittent avec le code 0, les erreurs d'utilisation avec 2 et les autres échecs avec 1. La commande `revert --to 0` est refusée comme erreur d'utilisation et affiche la commande `curl` exacte permettant d'effectuer cette opération.

**L'absence totale de JavaScript est le socle minimal, pas la limite.** La recherche fonctionne via un formulaire HTML classique, la création de profil et la saisie du code PIN sont des requêtes POST de formulaire natives, et même avec les scripts désactivés, tous les favoris restent visibles et cliquables. Des liens d'évitement intégrés permettent de sauter directement au contenu. Une couche d'amélioration progressive ajoute le volet d'ajout, l'édition sur place, le tri par glisser-déposer, la palette ⌘K et le fonctionnement hors ligne ; si cette couche venait à faillir, le fonctionnement sans JavaScript reste toujours opérationnel.

## Limites

- **Réseau local uniquement, sans HTTPS.** Le contenu des favoris est stocké sous forme de fichiers JSON en clair sur le disque ; placez un reverse proxy TLS en amont avant toute exposition sur Internet.
- **Un fichier `profile.json` corrompu bloque le profil concerné.** L'option `revert` ne peut pas le réparer ; `POST /api/v1/profiles/rebuild` (ou `launchbay profile rebuild`) réécrit uniquement ce fichier sans toucher au moindre octet des favoris, des instantanés ou de l'historique.
- **Délai d'attente pour la récupération des icônes** : un domaine ayant échoué ne sera pas réinterrogé pendant 24 heures. Ce délai évite de solliciter en boucle un site inaccessible.
- **Aucune liste partagée** : vous ne pouvez pas partager un favori avec toute la famille ni publier une page publiquement. Conserver chacun son propre espace est précisément le but recherché.
- **Pas encore validé sur un NAS réel.** Toutes les sessions de validation ont été menées sous Windows + Docker Desktop ; l'objectif de LCP sur réseau local, le temps de démarrage à froid, la fluidité de défilement avec 1 000 favoris et la latence du cache hors ligne restent encore à mesurer.
- **Aucun widget météo ou horloge.** Ils sortent du cadre de cette page. Les textes de pied de page et les données de favoris passent par `POST /api/v1/batch` ; la gestion des fichiers sur le disque appartient au serveur.
- **Les langues de l'interface couvrent l'interface utilisateur, pas les messages destinés aux scripts.** Le détail des erreurs d'API (`detail`), les motifs de validation des champs et le rapport d'importation lu par les scripts restent en chinois (le rapport affiché sur la page web adopte la langue de la page). Les programmes appelants ont besoin d'un texte stable et directement comparable ; vos titres de favoris, noms de groupes et adresses URL relèvent du contenu personnel et ne sont pas traduits non plus.
- **Les jetons sont vides par défaut**, ce qui désactive l'extension et la CLI tant que vous n'en ajoutez pas ; la clé d'administration fonctionne à l'inverse et est obligatoire dès le démarrage.

## D'où vient l'image

compose se contente de télécharger une image publiée ; la compilation relève de l'intégration continue (CI). Dès qu'un tag `v*` est posé (ou le workflow déclenché manuellement), `.github/workflows/docker.yml` compile les versions amd64 et arm64 et les publie sur Docker Hub et GHCR. Les tags de version sont `<version>`, `<major>.<minor>` et `<major>` (actuellement `0.4.0` / `0.4` / `0`), complétés par `latest` et le hash court du commit ; le numéro de version de l'image est exclusivement déterminé par le tag Git.

Pour lancer le service sans cloner le dépôt, démarrez directement l'image :

```bash
docker run -d --name launchbay -p 7600:7600 \
  -v launchbay-data:/data -e LAUNCHBAY_DATA_DIR=/data \
  -e LAUNCHBAY_ADMIN_TOKEN="votre-propre-cle-admin" \
  -e LAUNCHBAY_WRITE_TOKENS="ai-dad:votre-propre-cle:dad" -e TZ=Asia/Shanghai \
  --health-cmd "/launchbay healthcheck" --health-interval 30s \
  ghcr.io/rockbenben/launchbay:latest
```

## À propos du 365 Open Source Plan

Projet **#042** du [365 Open Source Plan](https://github.com/rockbenben/365opensource) — une personne + l'IA, plus de 300 projets open source en un an.

[Proposez votre idée →](https://365.aishort.top/) · [Discord](https://discord.gg/PZTQfJ4GjX) · [Telegram](https://t.me/aishort_top)
