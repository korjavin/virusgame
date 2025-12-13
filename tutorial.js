// Interactive Tutorial System
const Tutorial = {
    currentStep: 0,
    isActive: false,

    getSteps() {
        return [
            {
                title: i18n.t('tutorialWelcomeTitle'),
                content: i18n.t('tutorialWelcomeContent'),
                highlight: null
            },
            {
                title: i18n.t('tutorialBoardTitle'),
                content: i18n.t('tutorialBoardContent'),
                highlight: "#game-board"
            },
            {
                title: i18n.t('tutorialBaseTitle'),
                content: i18n.t('tutorialBaseContent'),
                highlight: ".player1-base"
            },
            {
                title: i18n.t('tutorialMovesTitle'),
                content: i18n.t('tutorialMovesContent'),
                highlight: "#status"
            },
            {
                title: i18n.t('tutorialGameModeTitle'),
                content: i18n.t('tutorialGameModeContent'),
                highlight: ".game-mode-toggle"
            },
            {
                title: i18n.t('tutorialLobbyTitle'),
                content: i18n.t('tutorialLobbyContent'),
                highlight: "#multiplayer-section",
                action: () => {
                    // Switch to multiplayer mode to show the lobby
                    document.getElementById('mode-multiplayer').click();
                }
            },
            {
                title: i18n.t('tutorialMultiplayerTitle'),
                content: i18n.t('tutorialMultiplayerContent'),
                highlight: ".users-section",
                action: () => {
                    // Switch back to 1v1 mode to show the online players list
                    document.getElementById('mode-1v1').click();
                }
            },
            {
                title: i18n.t('tutorialAITitle'),
                content: i18n.t('tutorialAIContent'),
                highlight: "#ai-enabled-group"
            },
            {
                title: i18n.t('tutorialReadyTitle'),
                content: i18n.t('tutorialReadyContent'),
                highlight: null
            }
        ];
    },

    start() {
        this.currentStep = 0;
        this.isActive = true;

        // Show tutorial modal and overlay
        // Use flex to enable the positioning logic in CSS
        document.getElementById('tutorial-modal').style.display = 'flex';
        // We do NOT show the overlay here because the highlight element itself
        // provides the dimming via a large box-shadow.
        // Showing #tutorial-overlay would double-darken the screen and obscure the highlighted area.
        document.getElementById('tutorial-overlay').style.display = 'none';

        this.showStep();
        this.setupEventListeners();
    },

    end() {
        this.isActive = false;

        // Hide all tutorial elements
        document.getElementById('tutorial-modal').style.display = 'none';
        document.getElementById('tutorial-overlay').style.display = 'none';
        document.getElementById('tutorial-highlight').style.display = 'none';

        this.currentStep = 0;
    },

    showStep() {
        const steps = this.getSteps();
        const step = steps[this.currentStep];
        const contentDiv = document.getElementById('tutorial-step-content');
        const progressDiv = document.getElementById('tutorial-progress');
        const prevBtn = document.getElementById('tutorial-prev-btn');
        const nextBtn = document.getElementById('tutorial-next-btn');

        // Update content
        contentDiv.innerHTML = `
            <h3>${step.title}</h3>
            <p>${step.content}</p>
        `;

        // Update progress indicator
        progressDiv.textContent = i18n.t('stepProgress', { current: this.currentStep + 1, total: steps.length });

        // Update button states
        prevBtn.disabled = this.currentStep === 0;
        prevBtn.textContent = i18n.t('previous');
        nextBtn.textContent = this.currentStep === steps.length - 1 ? i18n.t('finish') : i18n.t('next');

        // Execute step action if defined
        if (step.action) {
            step.action();
        }

        // Highlight element if specified
        // Delay slightly to allow UI updates (like switching tabs) to complete
        setTimeout(() => {
            this.highlightElement(step.highlight);

            // Scroll highlighted element into view
            if (step.highlight) {
                const element = document.querySelector(step.highlight);
                if (element) {
                    element.scrollIntoView({ behavior: 'smooth', block: 'center' });

                    // Check if element is low on screen and move modal to top if needed
                    const rect = element.getBoundingClientRect();
                    const windowHeight = window.innerHeight;
                    const modal = document.getElementById('tutorial-modal');

                    if (rect.bottom > windowHeight * 0.7) {
                        // Element is in bottom 30% of screen, move modal to top
                        modal.style.justifyContent = 'flex-start';
                        modal.style.paddingTop = '20px';
                        modal.style.paddingBottom = '0';
                    } else {
                        // Reset to bottom
                        modal.style.justifyContent = 'flex-end';
                        modal.style.paddingTop = '0';
                        modal.style.paddingBottom = '20px';
                    }
                }
            }
        }, 100);
    },

    highlightElement(selector) {
        const highlightDiv = document.getElementById('tutorial-highlight');
        const overlayDiv = document.getElementById('tutorial-overlay');

        if (!selector) {
            highlightDiv.style.display = 'none';
            // If no highlight, we might want to show the overlay to dim the background?
            // But currently the design seems to rely on highlight box-shadow.
            // If we are just showing a modal without highlight, maybe we should show overlay?
            // But let's keep it simple and consistent: if no highlight, no dimming (or full dimming?).
            // Let's assume 'null' highlight means general intro/outro, so maybe show overlay?
            overlayDiv.style.display = 'block';
            return;
        }

        // If we have a highlight, hide the full screen overlay because the highlight provides the dimming.
        overlayDiv.style.display = 'none';

        const element = document.querySelector(selector);
        if (!element) {
            highlightDiv.style.display = 'none';
            overlayDiv.style.display = 'block'; // Fallback to overlay if element not found
            return;
        }

        // Get element position and size
        const rect = element.getBoundingClientRect();
        const scrollTop = window.pageYOffset || document.documentElement.scrollTop;
        const scrollLeft = window.pageXOffset || document.documentElement.scrollLeft;

        // Position the highlight
        highlightDiv.style.display = 'block';
        highlightDiv.style.top = (rect.top + scrollTop - 10) + 'px';
        highlightDiv.style.left = (rect.left + scrollLeft - 10) + 'px';
        highlightDiv.style.width = (rect.width + 20) + 'px';
        highlightDiv.style.height = (rect.height + 20) + 'px';
    },

    nextStep() {
        const steps = this.getSteps();
        if (this.currentStep < steps.length - 1) {
            this.currentStep++;
            this.showStep();
        } else {
            // Tutorial complete
            this.end();
        }
    },

    prevStep() {
        if (this.currentStep > 0) {
            this.currentStep--;
            this.showStep();
        }
    },

    setupEventListeners() {
        const nextBtn = document.getElementById('tutorial-next-btn');
        const prevBtn = document.getElementById('tutorial-prev-btn');
        const skipBtn = document.getElementById('tutorial-skip-btn');

        // Remove old listeners by cloning (simple way to remove all listeners)
        const newNextBtn = nextBtn.cloneNode(true);
        const newPrevBtn = prevBtn.cloneNode(true);
        const newSkipBtn = skipBtn.cloneNode(true);

        nextBtn.parentNode.replaceChild(newNextBtn, nextBtn);
        prevBtn.parentNode.replaceChild(newPrevBtn, prevBtn);
        skipBtn.parentNode.replaceChild(newSkipBtn, skipBtn);

        // Update button text
        newSkipBtn.textContent = i18n.t('skipTutorial');

        // Add new listeners
        newNextBtn.addEventListener('click', () => this.nextStep());
        newPrevBtn.addEventListener('click', () => this.prevStep());
        newSkipBtn.addEventListener('click', () => this.end());

        // Update highlight on window resize
        window.addEventListener('resize', () => {
            if (this.isActive) {
                const steps = this.getSteps();
                const step = steps[this.currentStep];
                this.highlightElement(step.highlight);
            }
        });

        // Update highlight on scroll
        window.addEventListener('scroll', () => {
            if (this.isActive) {
                const steps = this.getSteps();
                const step = steps[this.currentStep];
                this.highlightElement(step.highlight);
            }
        });
    }
};

// Make Tutorial globally available
window.Tutorial = Tutorial;
