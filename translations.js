// Internationalization (i18n) system for Virus Game
const i18n = {
    currentLanguage: localStorage.getItem('language') || 'en',

    translations: {
        en: {
            // Page title
            pageTitle: 'Virus Game - Multiplayer',
            gameTitle: 'Virus Game',

            // Connection status
            connecting: 'Connecting...',
            connected: 'Connected',
            disconnected: 'Disconnected',

            // Sidebar sections
            gameSettings: 'Game Settings',
            onlinePlayers: 'Online Players',

            // Settings labels
            rows: 'Rows:',
            cols: 'Cols:',
            vsAI: 'vs AI',
            aiDepth: 'AI Depth:',
            aiTimeLimit: 'AI Time Limit (ms):',
            aiDepthTitle: 'How many moves ahead AI thinks (1-6)',
            aiTimeTitle: '0 = fixed depth, >0 = iterative deepening with time limit',
            showConnectionTrees: 'Show Connection Trees',
            newLocalGame: 'New Local Game',

            // AI Tuning
            aiStrategyWeights: 'AI Strategy Weights',
            aiTuningDesc: 'Optimized AI with 5 balanced parameters (all O(n)):',
            material: 'Material:',
            materialTitle: 'Cells and fortifications',
            mobility: 'Mobility:',
            mobilityTitle: 'Number of available moves',
            position: 'Position:',
            positionTitle: 'Aggression + attack opportunities',
            redundancy: 'Redundancy:',
            redundancyTitle: 'Network resilience (cells with 2+ connections)',
            cohesion: 'Cohesion:',
            cohesionTitle: 'Territory cohesion (penalize gaps/holes)',
            resetToDefaults: 'Reset to Defaults',

            // Online users
            noUsers: 'Connecting to server...',
            challenge: 'Challenge',

            // Game status
            yourTurn: 'Your turn ({symbol}) vs {opponent}. Moves left: {moves}.',
            opponentTurn: '{opponent}\'s turn. Waiting...',
            playerTurn: 'Player {player}\'s turn. Moves left: {moves}.',
            placeNeutral: 'Place {count} neutral field(s).',
            placeNeutralPlayer: 'Player {player}: Place {count} neutral field(s).',
            placeNeutralCancel: 'Place {count} neutral field(s). Click button again to cancel.',
            placeNeutralPlayerCancel: 'Player {player}: Place {count} neutral field(s). Click button again to cancel.',
            youWin: 'You win!',
            youLose: 'You lose!',
            playerWins: 'Player {player} wins!',
            noMoreMoves: 'Player {winner} wins! Player {player} has no more moves.',

            // AI progress
            aiThinking: 'AI thinking: {progress}',

            // Footer
            commit: 'Commit:',
            multiplayerVersion: 'Multiplayer v1.6',

            // Tooltips
            toggleDarkTheme: 'Toggle dark theme',
            gameRulesTutorial: 'Game Rules & Tutorial',
            changeLanguage: 'Change language / Sprache ändern',

            // Rules Modal
            rulesTitle: '🎮 Game Rules & How to Play',
            goal: '🎯 Goal',
            goalText: 'Eliminate all of your opponent\'s cells by spreading your virus across the board while protecting your base!',
            basicRules: '📜 Basic Rules',
            eachTurn: 'Each turn:',
            eachTurnText: 'You get 3 moves',
            validMoves: 'Valid moves:',
            validMovesText: 'Place your cell on an empty space or attack opponent\'s cell (must be adjacent to your territory and connected to your base)',
            attacking: 'Attacking:',
            attackingText: 'When you attack an opponent\'s cell, it becomes <strong>fortified</strong> (protected) and cannot be attacked again',
            winning: 'Winning:',
            winningText: 'The first player to eliminate all opponent\'s cells wins!',
            cellTypes: '🏰 Cell Types',
            baseCell: 'Base (green background):',
            baseCellText: 'Your starting cell - cannot be attacked and must always stay connected to your territory',
            regularCells: 'Regular cells (X or O):',
            regularCellsText: 'Your normal territory cells',
            fortifiedCells: 'Fortified cells (solid color):',
            fortifiedCellsText: 'Protected cells that cannot be attacked (created when you attack opponent\'s cell)',
            killedCells: 'Killed cells (gray):',
            killedCellsText: 'Neutral/dead cells that can be claimed by either player',
            playingWithAI: '🤖 Playing with AI',
            aiEnableText: 'Check "vs AI" in Game Settings to play against the computer',
            aiDepthText: 'Adjust AI Depth (1-6) to control difficulty',
            aiDepthExplain: 'Higher depth = smarter AI but slower moves',
            aiFineTune: 'Fine-tune AI strategy with AI Strategy Weights (for advanced players)',
            multiplayer: '👥 Multiplayer',
            multiplayerChallenge: 'Challenge:',
            multiplayerChallengeText: 'Click "Challenge" next to an online player\'s name',
            multiplayerAccept: 'Accept:',
            multiplayerAcceptText: 'When challenged, a notification will appear - click "Accept" to start',
            multiplayerYourTurn: 'Your turn:',
            multiplayerYourTurnText: 'The status will glow green when it\'s your turn',
            startTutorial: '📚 Start Interactive Tutorial',
            close: 'Close',

            // Tutorial steps
            tutorialWelcomeTitle: 'Welcome to Virus Game! 👋',
            tutorialWelcomeContent: 'This interactive tutorial will teach you how to play. Click \'Next\' to continue or \'Skip\' to exit.',
            tutorialBoardTitle: 'The Game Board 🎮',
            tutorialBoardContent: 'This is the game board where all the action happens. You\'ll place your cells (X) here to spread your virus and defeat your opponent (O).',
            tutorialBaseTitle: 'Your Base 🏰',
            tutorialBaseContent: 'The green cell in the top-left corner is YOUR BASE (Player 1 - X). It cannot be attacked and all your cells must stay connected to it. Your opponent\'s base is in the bottom-right corner.',
            tutorialMovesTitle: 'Making Moves ⚡',
            tutorialMovesContent: 'You get 3 moves per turn. Click on empty cells adjacent to your territory to expand, or click on opponent\'s cells to attack them. All moves must connect to your base!',
            tutorialFortifiedTitle: 'Fortified Cells 🛡️',
            tutorialFortifiedContent: 'When you attack an opponent\'s cell, it becomes FORTIFIED (solid color). Fortified cells cannot be attacked again - they\'re permanent!',
            tutorialSettingsTitle: 'Game Settings ⚙️',
            tutorialSettingsContent: 'Here you can customize the board size, enable AI opponent, and start a new game. Try adjusting these settings to match your skill level!',

            // New tutorial keys
            tutorialGameModeTitle: 'Game Mode Selection 🎮',
            tutorialGameModeContent: 'Choose "1v1" for local play or "Multiplayer" to play online. This button switches the mode.',
            tutorialLobbyTitle: 'Multiplayer Lobby 🏢',
            tutorialLobbyContent: 'In multiplayer mode, you can create a new lobby or join an existing one from the list.',

            tutorialAITitle: 'Playing with AI 🤖',
            tutorialAIContent: 'Check \'vs AI\' to practice against the computer. Adjust the AI Depth to make it easier (1-2) or harder (4-6). The AI will play as Player 2 (O).',
            tutorialAITuningTitle: 'AI Strategy Tuning 🎛️',
            tutorialAITuningContent: 'For advanced players: you can fine-tune the AI\'s strategy by adjusting these weights. Material controls how the AI values cells, Mobility affects positioning, etc.',
            tutorialMultiplayerTitle: 'Online Players 👥',
            tutorialMultiplayerContent: 'See online players here! Click \'Challenge\' next to a player\'s name to invite them to a game.',
            tutorialNotificationsTitle: 'Challenge Notifications 📬',
            tutorialNotificationsContent: 'When someone challenges you, a notification will appear in the top-right corner. Click \'Accept\' to start playing or \'Decline\' to refuse.',
            tutorialTurnIndicatorTitle: 'Your Turn Indicator 💚',
            tutorialTurnIndicatorContent: 'During multiplayer games, the status text will glow green when it\'s YOUR turn. Make your moves quickly to keep the game flowing!',
            tutorialDarkThemeTitle: 'Dark Theme 🌙',
            tutorialDarkThemeContent: 'Prefer dark mode? Click this moon icon to toggle between light and dark themes. Your eyes will thank you during late-night gaming sessions!',
            tutorialHelpTitle: 'Need Help? ❓',
            tutorialHelpContent: 'Click this help button anytime to review the rules or restart this tutorial. You\'re now ready to play!',
            tutorialReadyTitle: 'Ready to Play! 🎉',
            tutorialReadyContent: 'You\'ve completed the tutorial! Start a local game to practice, enable AI for a challenge, or go online to compete with real players. Good luck!',

            // Tutorial controls
            previous: '← Previous',
            next: 'Next →',
            finish: 'Finish 🎉',
            skipTutorial: 'Skip Tutorial',
            stepProgress: 'Step {current} of {total}'
        },

        de: {
            // Page title
            pageTitle: 'Virus-Spiel - Multiplayer',
            gameTitle: 'Virus-Spiel',

            // Connection status
            connecting: 'Verbinden...',
            connected: 'Verbunden',
            disconnected: 'Getrennt',

            // Sidebar sections
            gameSettings: 'Spieleinstellungen',
            onlinePlayers: 'Online-Spieler',

            // Settings labels
            rows: 'Zeilen:',
            cols: 'Spalten:',
            vsAI: 'gegen KI',
            aiDepth: 'KI-Tiefe:',
            aiTimeLimit: 'KI-Zeitlimit (ms):',
            aiDepthTitle: 'Wie viele Züge die KI vorausdenkt (1-6)',
            aiTimeTitle: '0 = feste Tiefe, >0 = iterative Vertiefung mit Zeitlimit',
            showConnectionTrees: 'Verbindungslinien anzeigen',
            newLocalGame: 'Neues lokales Spiel',

            // AI Tuning
            aiStrategyWeights: 'KI-Strategiegewichte',
            aiTuningDesc: 'Optimierte KI mit 5 ausgewogenen Parametern (alle O(n)):',
            material: 'Material:',
            materialTitle: 'Zellen und Befestigungen',
            mobility: 'Mobilität:',
            mobilityTitle: 'Anzahl verfügbarer Züge',
            position: 'Position:',
            positionTitle: 'Aggression + Angriffsmöglichkeiten',
            redundancy: 'Redundanz:',
            redundancyTitle: 'Netzwerk-Resilienz (Zellen mit 2+ Verbindungen)',
            cohesion: 'Kohäsion:',
            cohesionTitle: 'Gebiets-Kohäsion (Lücken/Löcher bestrafen)',
            resetToDefaults: 'Auf Standard zurücksetzen',

            // Online users
            noUsers: 'Verbinde mit Server...',
            challenge: 'Herausfordern',

            // Game status
            yourTurn: 'Du bist dran ({symbol}) gegen {opponent}. Züge übrig: {moves}.',
            opponentTurn: '{opponent} ist dran. Warten...',
            playerTurn: 'Spieler {player} ist dran. Züge übrig: {moves}.',
            placeNeutral: 'Platziere {count} neutrale(s) Feld(er).',
            placeNeutralPlayer: 'Spieler {player}: Platziere {count} neutrale(s) Feld(er).',
            placeNeutralCancel: 'Platziere {count} neutrale(s) Feld(er). Klicke erneut auf die Schaltfläche, um abzubrechen.',
            placeNeutralPlayerCancel: 'Spieler {player}: Platziere {count} neutrale(s) Feld(er). Klicke erneut auf die Schaltfläche, um abzubrechen.',
            youWin: 'Du gewinnst!',
            youLose: 'Du verlierst!',
            playerWins: 'Spieler {player} gewinnt!',
            noMoreMoves: 'Spieler {winner} gewinnt! Spieler {player} hat keine Züge mehr.',

            // AI progress
            aiThinking: 'KI denkt nach: {progress}',

            // Footer
            commit: 'Commit:',
            multiplayerVersion: 'Multiplayer v1.6',

            // Tooltips
            toggleDarkTheme: 'Dunkles Design umschalten',
            gameRulesTutorial: 'Spielregeln & Tutorial',
            changeLanguage: 'Sprache ändern / Change language',

            // Rules Modal
            rulesTitle: '🎮 Spielregeln & Anleitung',
            goal: '🎯 Ziel',
            goalText: 'Eliminiere alle Zellen deines Gegners, indem du dein Virus über das Spielfeld verbreitest und gleichzeitig deine Basis schützt!',
            basicRules: '📜 Grundregeln',
            eachTurn: 'Jeder Zug:',
            eachTurnText: 'Du bekommst 3 Züge',
            validMoves: 'Gültige Züge:',
            validMovesText: 'Platziere deine Zelle auf einem leeren Feld oder greife die Zelle des Gegners an (muss an dein Gebiet angrenzen und mit deiner Basis verbunden sein)',
            attacking: 'Angreifen:',
            attackingText: 'Wenn du die Zelle eines Gegners angreifst, wird sie <strong>befestigt</strong> (geschützt) und kann nicht erneut angegriffen werden',
            winning: 'Gewinnen:',
            winningText: 'Der erste Spieler, der alle gegnerischen Zellen eliminiert, gewinnt!',
            cellTypes: '🏰 Zellentypen',
            baseCell: 'Basis (grüner Hintergrund):',
            baseCellText: 'Deine Startzelle - kann nicht angegriffen werden und muss immer mit deinem Gebiet verbunden bleiben',
            regularCells: 'Normale Zellen (X oder O):',
            regularCellsText: 'Deine normalen Gebietszellen',
            fortifiedCells: 'Befestigte Zellen (Vollfarbe):',
            fortifiedCellsText: 'Geschützte Zellen, die nicht angegriffen werden können (entstehen, wenn du die Zelle eines Gegners angreifst)',
            killedCells: 'Getötete Zellen (grau):',
            killedCellsText: 'Neutrale/tote Zellen, die von beiden Spielern beansprucht werden können',
            playingWithAI: '🤖 Gegen die KI spielen',
            aiEnableText: 'Aktiviere "gegen KI" in den Spieleinstellungen, um gegen den Computer zu spielen',
            aiDepthText: 'Passe die KI-Tiefe (1-6) an, um den Schwierigkeitsgrad zu steuern',
            aiDepthExplain: 'Höhere Tiefe = intelligentere KI, aber langsamere Züge',
            aiFineTune: 'Feinabstimmung der KI-Strategie mit KI-Strategiegewichten (für fortgeschrittene Spieler)',
            multiplayer: '👥 Mehrspieler',
            multiplayerChallenge: 'Herausfordern:',
            multiplayerChallengeText: 'Klicke auf "Herausfordern" neben dem Namen eines Online-Spielers',
            multiplayerAccept: 'Akzeptieren:',
            multiplayerAcceptText: 'Wenn du herausgefordert wirst, erscheint eine Benachrichtigung - klicke auf "Akzeptieren" um zu starten',
            multiplayerYourTurn: 'Dein Zug:',
            multiplayerYourTurnText: 'Der Status leuchtet grün, wenn du an der Reihe bist',
            startTutorial: '📚 Interaktives Tutorial starten',
            close: 'Schließen',

            // Tutorial steps
            tutorialWelcomeTitle: 'Willkommen beim Virus-Spiel! 👋',
            tutorialWelcomeContent: 'Dieses interaktive Tutorial zeigt dir, wie man spielt. Klicke auf \'Weiter\', um fortzufahren, oder auf \'Überspringen\', um zu beenden.',
            tutorialBoardTitle: 'Das Spielfeld 🎮',
            tutorialBoardContent: 'Dies ist das Spielfeld, auf dem die gesamte Aktion stattfindet. Du platzierst deine Zellen (X) hier, um dein Virus zu verbreiten und deinen Gegner (O) zu besiegen.',
            tutorialBaseTitle: 'Deine Basis 🏰',
            tutorialBaseContent: 'Die grüne Zelle in der oberen linken Ecke ist DEINE BASIS (Spieler 1 - X). Sie kann nicht angegriffen werden und alle deine Zellen müssen mit ihr verbunden bleiben. Die Basis deines Gegners ist in der unteren rechten Ecke.',
            tutorialMovesTitle: 'Züge machen ⚡',
            tutorialMovesContent: 'Du bekommst 3 Züge pro Runde. Klicke auf leere Felder neben deinem Gebiet, um zu expandieren, oder klicke auf gegnerische Zellen, um sie anzugreifen. Alle Züge müssen mit deiner Basis verbunden sein!',
            tutorialFortifiedTitle: 'Befestigte Zellen 🛡️',
            tutorialFortifiedContent: 'Wenn du eine gegnerische Zelle angreifst, wird sie BEFESTIGT (Vollfarbe). Befestigte Zellen können nicht erneut angegriffen werden - sie sind permanent!',
            tutorialSettingsTitle: 'Spieleinstellungen ⚙️',
            tutorialSettingsContent: 'Hier kannst du die Spielfeldgröße anpassen, einen KI-Gegner aktivieren und ein neues Spiel starten. Passe diese Einstellungen an dein Können an!',

            // New tutorial keys
            tutorialGameModeTitle: 'Spielmodus-Auswahl 🎮',
            tutorialGameModeContent: 'Wähle "1v1" für lokales Spiel oder "Multiplayer" für Online-Spiele. Dieser Schalter wechselt den Modus.',
            tutorialLobbyTitle: 'Mehrspieler-Lobby 🏢',
            tutorialLobbyContent: 'Im Mehrspielermodus kannst du eine neue Lobby erstellen oder einer bestehenden aus der Liste beitreten.',

            tutorialAITitle: 'Gegen die KI spielen 🤖',
            tutorialAIContent: 'Aktiviere \'gegen KI\', um gegen den Computer zu üben. Passe die KI-Tiefe an, um es einfacher (1-2) oder schwerer (4-6) zu machen. Die KI spielt als Spieler 2 (O).',
            tutorialAITuningTitle: 'KI-Strategie-Feinabstimmung 🎛️',
            tutorialAITuningContent: 'Für fortgeschrittene Spieler: Du kannst die Strategie der KI durch Anpassung dieser Gewichte feinabstimmen. Material steuert, wie die KI Zellen bewertet, Mobilität beeinflusst die Positionierung usw.',
            tutorialMultiplayerTitle: 'Online-Spieler 👥',
            tutorialMultiplayerContent: 'Hier siehst du Online-Spieler! Klicke auf \'Herausfordern\' neben dem Namen eines Spielers, um ihn zu einem Spiel einzuladen.',
            tutorialNotificationsTitle: 'Herausforderungs-Benachrichtigungen 📬',
            tutorialNotificationsContent: 'Wenn dich jemand herausfordert, erscheint eine Benachrichtigung in der oberen rechten Ecke. Klicke auf \'Akzeptieren\', um zu spielen, oder auf \'Ablehnen\'.',
            tutorialTurnIndicatorTitle: 'Dein Zug-Indikator 💚',
            tutorialTurnIndicatorContent: 'Während Mehrspieler-Spielen leuchtet der Statustext grün, wenn DU an der Reihe bist. Mache deine Züge schnell, um das Spiel am Laufen zu halten!',
            tutorialDarkThemeTitle: 'Dunkles Design 🌙',
            tutorialDarkThemeContent: 'Bevorzugst du den dunklen Modus? Klicke auf dieses Mondsymbol, um zwischen hellen und dunklen Designs umzuschalten. Deine Augen werden es dir danken!',
            tutorialHelpTitle: 'Brauchst du Hilfe? ❓',
            tutorialHelpContent: 'Klicke jederzeit auf diese Hilfe-Schaltfläche, um die Regeln zu überprüfen oder dieses Tutorial neu zu starten. Du bist jetzt bereit zu spielen!',
            tutorialReadyTitle: 'Bereit zu spielen! 🎉',
            tutorialReadyContent: 'Du hast das Tutorial abgeschlossen! Starte ein lokales Spiel zum Üben, aktiviere die KI für eine Herausforderung oder spiele online gegen echte Spieler. Viel Glück!',

            // Tutorial controls
            previous: '← Zurück',
            next: 'Weiter →',
            finish: 'Fertig 🎉',
            skipTutorial: 'Tutorial überspringen',
            stepProgress: 'Schritt {current} von {total}'
        }
    },

    // Get translated text
    t(key, replacements = {}) {
        let text = this.translations[this.currentLanguage][key] || key;

        // Replace placeholders like {symbol}, {opponent}, etc.
        Object.keys(replacements).forEach(placeholder => {
            text = text.replace(new RegExp(`\\{${placeholder}\\}`, 'g'), replacements[placeholder]);
        });

        return text;
    },

    // Set language and update UI
    setLanguage(lang) {
        if (this.translations[lang]) {
            this.currentLanguage = lang;
            localStorage.setItem('language', lang);
            this.updatePageContent();

            // Dispatch event for other scripts to listen to
            window.dispatchEvent(new CustomEvent('languageChanged', { detail: { language: lang } }));
        }
    },

    // Update all page content with current language
    updatePageContent() {
        // Update page title
        document.title = this.t('pageTitle');

        // Update game title
        const gameTitle = document.querySelector('#game-container h1');
        if (gameTitle) gameTitle.textContent = this.t('gameTitle');

        // Update sidebar sections
        const sections = document.querySelectorAll('.sidebar-section h3');
        if (sections[0]) sections[0].textContent = this.t('gameSettings');
        if (sections[2]) sections[2].textContent = this.t('onlinePlayers');

        // Update settings labels
        const labels = {
            'rows-input': 'rows',
            'cols-input': 'cols',
            'ai-depth-input': 'aiDepth',
            'ai-time-input': 'aiTimeLimit'
        };

        Object.keys(labels).forEach(id => {
            const input = document.getElementById(id);
            if (input) {
                const label = input.previousElementSibling || input.parentElement.querySelector('label');
                if (label && label.tagName === 'LABEL') {
                    const parts = label.textContent.split(':');
                    if (parts.length > 0) {
                        label.childNodes[0].textContent = this.t(labels[id]);
                    }
                }
            }
        });

        // Update AI checkbox label
        const aiLabel = document.querySelector('label[for="ai-enabled"]');
        if (aiLabel) {
            const checkbox = aiLabel.querySelector('input');
            aiLabel.childNodes.forEach(node => {
                if (node.nodeType === Node.TEXT_NODE && node.textContent.trim()) {
                    node.textContent = ' ' + this.t('vsAI');
                }
            });
        }

        // Update Connection Tree checkbox label
        const showConnectionsSpan = document.querySelector('span[data-translation-key="showConnectionTrees"]');
        if (showConnectionsSpan) {
            showConnectionsSpan.textContent = this.t('showConnectionTrees');
        }

        // Update buttons
        const newGameBtn = document.getElementById('new-game-button');
        if (newGameBtn) newGameBtn.textContent = this.t('newLocalGame');

        const resetCoeffsBtn = document.getElementById('reset-coeffs-button');
        if (resetCoeffsBtn) resetCoeffsBtn.textContent = this.t('resetToDefaults');

        // Update AI tuning section
        const aiTuningHeader = document.getElementById('ai-tuning-header');
        if (aiTuningHeader) {
            const arrow = aiTuningHeader.querySelector('span') ? aiTuningHeader.querySelector('span').textContent : '▼';
            aiTuningHeader.innerHTML = `⚙️ ${this.t('aiStrategyWeights')} <span style="float: right;">${arrow}</span>`;
        }

        // Update tooltips
        const themeToggle = document.getElementById('theme-toggle');
        if (themeToggle) themeToggle.title = this.t('toggleDarkTheme');

        const helpButton = document.getElementById('help-button');
        if (helpButton) helpButton.title = this.t('gameRulesTutorial');

        const langToggle = document.getElementById('lang-toggle');
        if (langToggle) langToggle.title = this.t('changeLanguage');

        // Update input titles
        const aiDepthInput = document.getElementById('ai-depth-input');
        if (aiDepthInput) aiDepthInput.title = this.t('aiDepthTitle');

        const aiTimeInput = document.getElementById('ai-time-input');
        if (aiTimeInput) aiTimeInput.title = this.t('aiTimeTitle');

        // Update Rules Modal
        this.updateRulesModal();

        // Update status if exists
        if (typeof updateStatus === 'function') {
            updateStatus();
        }

        // Update tutorial if active
        if (window.Tutorial && window.Tutorial.isActive) {
            window.Tutorial.showStep();
        }
    },

    // Update Rules Modal content
    updateRulesModal() {
        const rulesModal = document.getElementById('rules-modal');
        if (!rulesModal) return;

        const modalContent = rulesModal.querySelector('.modal-content');
        if (!modalContent) return;

        modalContent.innerHTML = `
            <span class="modal-close" id="rules-close">&times;</span>
            <h2>${this.t('rulesTitle')}</h2>

            <div style="text-align: center; margin-bottom: 20px;">
                <button id="start-tutorial-btn" class="primary-btn" style="width: 100%; max-width: 300px;">${this.t('startTutorial')}</button>
            </div>

            <div class="modal-section">
                <h3>${this.t('goal')}</h3>
                <p>${this.t('goalText')}</p>
            </div>

            <div class="modal-section">
                <h3>${this.t('basicRules')}</h3>
                <ul>
                    <li><strong>${this.t('eachTurn')}</strong> ${this.t('eachTurnText')}</li>
                    <li><strong>${this.t('validMoves')}</strong> ${this.t('validMovesText')}</li>
                    <li><strong>${this.t('attacking')}</strong> ${this.t('attackingText')}</li>
                    <li><strong>${this.t('winning')}</strong> ${this.t('winningText')}</li>
                </ul>
            </div>

            <div class="modal-section">
                <h3>${this.t('cellTypes')}</h3>
                <ul>
                    <li><strong>${this.t('baseCell')}</strong> ${this.t('baseCellText')}</li>
                    <li><strong>${this.t('regularCells')}</strong> ${this.t('regularCellsText')}</li>
                    <li><strong>${this.t('fortifiedCells')}</strong> ${this.t('fortifiedCellsText')}</li>
                    <li><strong>${this.t('killedCells')}</strong> ${this.t('killedCellsText')}</li>
                </ul>
            </div>

            <div class="modal-section">
                <h3>${this.t('playingWithAI')}</h3>
                <ul>
                    <li>${this.t('aiEnableText')}</li>
                    <li>${this.t('aiDepthText')}</li>
                    <li>${this.t('aiDepthExplain')}</li>
                    <li>${this.t('aiFineTune')}</li>
                </ul>
            </div>

            <div class="modal-section">
                <h3>${this.t('multiplayer')}</h3>
                <ul>
                    <li><strong>${this.t('multiplayerChallenge')}</strong> ${this.t('multiplayerChallengeText')}</li>
                    <li><strong>${this.t('multiplayerAccept')}</strong> ${this.t('multiplayerAcceptText')}</li>
                    <li><strong>${this.t('multiplayerYourTurn')}</strong> ${this.t('multiplayerYourTurnText')}</li>
                </ul>
            </div>

            <div class="modal-actions">
                <button id="close-rules-btn" class="secondary-btn">${this.t('close')}</button>
            </div>
        `;

        // Re-attach event listeners
        document.getElementById('rules-close').addEventListener('click', function() {
            document.getElementById('rules-modal').style.display = 'none';
        });

        document.getElementById('close-rules-btn').addEventListener('click', function() {
            document.getElementById('rules-modal').style.display = 'none';
        });

        document.getElementById('start-tutorial-btn').addEventListener('click', function() {
            document.getElementById('rules-modal').style.display = 'none';
            if (typeof Tutorial !== 'undefined') Tutorial.start();
        });
    }
};

// Initialize language on page load
document.addEventListener('DOMContentLoaded', () => {
    i18n.updatePageContent();
});

// Make i18n globally available
window.i18n = i18n;
