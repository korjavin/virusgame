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
                title: i18n.t('tutorialFortifiedTitle'),
                content: i18n.t('tutorialFortifiedContent'),
                highlight: "#game-board"
            },
            {
                title: i18n.t('tutorialSettingsTitle'),
                content: i18n.t('tutorialSettingsContent'),
                highlight: ".sidebar-section:first-of-type"
            },
            {
                title: i18n.t('tutorialAITitle'),
                content: i18n.t('tutorialAIContent'),
                highlight: "#ai-enabled"
            },
            {
                title: i18n.t('tutorialAITuningTitle'),
                content: i18n.t('tutorialAITuningContent'),
                highlight: "#ai-tuning-section"
            },
            {
                title: i18n.t('tutorialMultiplayerTitle'),
                content: i18n.t('tutorialMultiplayerContent'),
                highlight: ".users-section"
            },
            {
                title: i18n.t('tutorialNotificationsTitle'),
                content: i18n.t('tutorialNotificationsContent'),
                highlight: "#notifications"
            },
            {
                title: i18n.t('tutorialTurnIndicatorTitle'),
                content: i18n.t('tutorialTurnIndicatorContent'),
                highlight: "#status"
            },
            {
                title: i18n.t('tutorialDarkThemeTitle'),
                content: i18n.t('tutorialDarkThemeContent'),
                highlight: "#theme-toggle"
            },
            {
                title: i18n.t('tutorialHelpTitle'),
                content: i18n.t('tutorialHelpContent'),
                highlight: "#help-button"
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
        document.getElementById('tutorial-modal').style.display = 'block';
        document.getElementById('tutorial-overlay').style.display = 'block';

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

        // Highlight element if specified
        this.highlightElement(step.highlight);

        // Scroll highlighted element into view
        if (step.highlight) {
            const element = document.querySelector(step.highlight);
            if (element) {
                setTimeout(() => {
                    element.scrollIntoView({ behavior: 'smooth', block: 'center' });
                }, 300);
            }
        }
    },

    highlightElement(selector) {
        const highlightDiv = document.getElementById('tutorial-highlight');

        if (!selector) {
            highlightDiv.style.display = 'none';
            return;
        }

        const element = document.querySelector(selector);
        if (!element) {
            highlightDiv.style.display = 'none';
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
