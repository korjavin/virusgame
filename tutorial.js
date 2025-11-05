// Interactive Tutorial System
const Tutorial = {
    currentStep: 0,
    isActive: false,

    steps: [
        {
            title: "Welcome to Virus Game! 👋",
            content: "This interactive tutorial will teach you how to play. Click 'Next' to continue or 'Skip' to exit.",
            highlight: null
        },
        {
            title: "The Game Board 🎮",
            content: "This is the game board where all the action happens. You'll place your cells (X) here to spread your virus and defeat your opponent (O).",
            highlight: "#game-board"
        },
        {
            title: "Your Base 🏰",
            content: "The green cell in the top-left corner is YOUR BASE (Player 1 - X). It cannot be attacked and all your cells must stay connected to it. Your opponent's base is in the bottom-right corner.",
            highlight: ".player1-base"
        },
        {
            title: "Making Moves ⚡",
            content: "You get 3 moves per turn. Click on empty cells adjacent to your territory to expand, or click on opponent's cells to attack them. All moves must connect to your base!",
            highlight: "#status"
        },
        {
            title: "Fortified Cells 🛡️",
            content: "When you attack an opponent's cell, it becomes FORTIFIED (solid color). Fortified cells cannot be attacked again - they're permanent!",
            highlight: "#game-board"
        },
        {
            title: "Game Settings ⚙️",
            content: "Here you can customize the board size, enable AI opponent, and start a new game. Try adjusting these settings to match your skill level!",
            highlight: ".sidebar-section:first-of-type"
        },
        {
            title: "Playing with AI 🤖",
            content: "Check 'vs AI' to practice against the computer. Adjust the AI Depth to make it easier (1-2) or harder (4-6). The AI will play as Player 2 (O).",
            highlight: "#ai-enabled"
        },
        {
            title: "AI Strategy Tuning 🎛️",
            content: "For advanced players: you can fine-tune the AI's strategy by adjusting these weights. Material controls how the AI values cells, Mobility affects positioning, etc.",
            highlight: "#ai-tuning-section"
        },
        {
            title: "Multiplayer Mode 👥",
            content: "See online players here! Click 'Challenge' next to a player's name to invite them to a game. Accept challenges when you receive notifications.",
            highlight: ".users-section"
        },
        {
            title: "Challenge Notifications 📬",
            content: "When someone challenges you, a notification will appear in the top-right corner. Click 'Accept' to start playing or 'Decline' to refuse.",
            highlight: "#notifications"
        },
        {
            title: "Your Turn Indicator 💚",
            content: "During multiplayer games, the status text will glow green when it's YOUR turn. Make your moves quickly to keep the game flowing!",
            highlight: "#status"
        },
        {
            title: "Dark Theme 🌙",
            content: "Prefer dark mode? Click this moon icon to toggle between light and dark themes. Your eyes will thank you during late-night gaming sessions!",
            highlight: "#theme-toggle"
        },
        {
            title: "Need Help? ❓",
            content: "Click this help button anytime to review the rules or restart this tutorial. You're now ready to play!",
            highlight: "#help-button"
        },
        {
            title: "Ready to Play! 🎉",
            content: "You've completed the tutorial! Start a local game to practice, enable AI for a challenge, or go online to compete with real players. Good luck!",
            highlight: null
        }
    ],

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
        const step = this.steps[this.currentStep];
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
        progressDiv.textContent = `Step ${this.currentStep + 1} of ${this.steps.length}`;

        // Update button states
        prevBtn.disabled = this.currentStep === 0;
        nextBtn.textContent = this.currentStep === this.steps.length - 1 ? 'Finish 🎉' : 'Next →';

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
        if (this.currentStep < this.steps.length - 1) {
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

        // Add new listeners
        newNextBtn.addEventListener('click', () => this.nextStep());
        newPrevBtn.addEventListener('click', () => this.prevStep());
        newSkipBtn.addEventListener('click', () => this.end());

        // Update highlight on window resize
        window.addEventListener('resize', () => {
            if (this.isActive) {
                const step = this.steps[this.currentStep];
                this.highlightElement(step.highlight);
            }
        });

        // Update highlight on scroll
        window.addEventListener('scroll', () => {
            if (this.isActive) {
                const step = this.steps[this.currentStep];
                this.highlightElement(step.highlight);
            }
        });
    }
};

// Make Tutorial globally available
window.Tutorial = Tutorial;
