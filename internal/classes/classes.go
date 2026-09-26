// Package classes is the compile-time catalogue of the twelve canonical
// Dungeons & Dragons 5e classes: display data (symbol, blurbs, lore) plus the
// matching hints Jev scores professions against. Original copy throughout —
// D&D is Wizards of the Coast's trademark and this is an unaffiliated fan toy.
package classes

// Class is one playable class: everything the UI shows and Jev weighs.
type Class struct {
	ID          string    // stable key, also the Jev option key
	Name        string    // display name
	Symbol      string    // emoji crest glyph
	Blurb       string    // one-liner for the runner-up cards
	Description string    // the static lore on the winner card
	Traits      []string  // trait chips
	Quirks      [6]string // one original character quirk per d6 face
	Criteria    string    // profession-matching hints for Jev
}

// All lists the twelve 5e classes in canonical order; catalog order is also
// the tie-break when two classes score equally.
var All = []Class{
	{
		ID:     "barbarian",
		Name:   "Barbarian",
		Symbol: "🪓",
		Blurb:  "Rage first, questions later.",
		Description: "You don't solve problems so much as remove them. " +
			"Where others see a locked door, you see a door that has made a terrible mistake. " +
			"Your diplomacy is intimidation, your armor is anger, and somewhere inside you is a " +
			"well of fury that no amount of therapy has managed to drain.",
		Traits: []string{"Rage", "Unarmored Defense", "Reckless Attack", "Big Hits"},
		Quirks: [6]string{
			"Names every axe. Remembers the axes' birthdays.",
			"Calls every locked door a personal challenge.",
			"Carries emergency snacks for post-rage recovery.",
			"Apologizes to the furniture after a fight.",
			"Knits between battles. The needles are enormous.",
			"Challenges thunderstorms to arm-wrestling.",
		},
		Criteria: "primal fury, raw strength, explosive temperament; fits lumberjacks, bouncers, " +
			"powerlifters, athletes, demolition crews, anyone ever told to calm down",
	},
	{
		ID:     "bard",
		Name:   "Bard",
		Symbol: "🎸",
		Blurb:  "Talks their way out — or into — anything.",
		Description: "You believe every problem is a performance problem. " +
			"You play six instruments, three of them well, and you have never met a stranger who " +
			"stayed a stranger. Your magic is charisma with a chord progression, and the party " +
			"pretends to be embarrassed by your ballads while secretly requesting encores.",
		Traits: []string{"Bardic Inspiration", "Jack of All Trades", "Vicious Mockery", "Storytelling"},
		Quirks: [6]string{
			"Turns introductions into suspiciously catchy jingles.",
			"Writes diss tracks about every defeated villain.",
			"Has a ballad for every occasion, including breakfast.",
			"Insists the dungeon has excellent acoustics.",
			"Keeps a running list of taverns that owe an encore.",
			"Claims every dramatic entrance was carefully rehearsed.",
		},
		Criteria: "charm, performance, wit, storytelling; fits musicians, comedians, teachers, " +
			"salespeople, anyone who talks for a living",
	},
	{
		ID:     "cleric",
		Name:   "Cleric",
		Symbol: "⚕️",
		Blurb:  "The party's healer — and its conscience.",
		Description: "Someone has to keep these disaster gremlins alive, and fate picked you. " +
			"You channel divine power through devotion, which is a polite way of saying you argue " +
			"with a god and usually win. Your friends are chaos incarnate; your patience is the " +
			"real miracle.",
		Traits: []string{"Channel Divinity", "Healing Word", "Turn Undead", "Moral Compass"},
		Quirks: [6]string{
			"Sends their deity extremely detailed incident reports.",
			"Labels all healing potions with encouraging notes.",
			"Offers blessings, bandages, and unsolicited life advice.",
			"Maintains a prayer for finding everyone's missing socks.",
			"Has threatened to put the entire party on bed rest.",
			"Carries a tiny bell for divine customer support.",
		},
		Criteria: "healing, faith, devotion, caretaking; fits nurses, doctors, paramedics, clergy, " +
			"social workers, teachers, parents",
	},
	{
		ID:     "druid",
		Name:   "Druid",
		Symbol: "🌿",
		Blurb:  "Turns into a bear. Solves everything as a bear.",
		Description: "You read the room better when the room is a forest. " +
			"You can shapeshift into any beast you have studied, which mostly means bear, and your " +
			"idea of networking is talking to ravens. Nature is your church, your gym, and your " +
			"group chat.",
		Traits: []string{"Wild Shape", "Entangle", "Goodberry", "Bear Mode"},
		Quirks: [6]string{
			"Introduces every houseplant as an old friend.",
			"Turns into a badger to avoid awkward conversations.",
			"Has a raven who gives terrible relationship advice.",
			"Brings moss to meetings. Calls it moral support.",
			"Judges inns by the quality of their window-box herbs.",
			"Insists the bear form needs its own seat at dinner.",
		},
		Criteria: "nature, shapeshifting, stewardship; fits farmers, biologists, park rangers, " +
			"veterinarians, ecologists, gardeners",
	},
	{
		ID:     "fighter",
		Name:   "Fighter",
		Symbol: "⚔️",
		Blurb:  "The simplest solution, executed perfectly.",
		Description: "While the wizard studied the arcane and the warlock made questionable deals, " +
			"you practiced. Then you practiced more. You are the party's Swiss army knife of " +
			"violence: every weapon, every tactic, every hold-my-beer moment. Consistency is your " +
			"superpower.",
		Traits: []string{"Action Surge", "Second Wind", "Extra Attack", "Weapon Master"},
		Quirks: [6]string{
			"Sharpens swords while making excellent small talk.",
			"Turns every chore into a training montage.",
			"Rates chairs by their usefulness as improvised weapons.",
			"Always has a backup plan and a backup sword.",
			"Practices heroic poses in reflective shields.",
			"Treats initiative rolls like a competitive sport.",
		},
		Criteria: "discipline, weapons mastery, tactical grit; fits soldiers, police, athletes, " +
			"martial artists, fencers, guards, coaches",
	},
	{
		ID:     "monk",
		Name:   "Monk",
		Symbol: "👊",
		Blurb:  "Weapon? These hands ARE the weapon.",
		Description: "Your body is a temple and you have weaponized the temple. " +
			"Years of discipline turned your fists into magical instruments and your patience " +
			"into something genuinely frightening. You move like water, hit like a collapsing wall, " +
			"and always have a cryptic proverb ready.",
		Traits: []string{"Flurry of Blows", "Stunning Strike", "Step of the Wind", "Inner Peace-ish"},
		Quirks: [6]string{
			"Can meditate through anything except chewing noises.",
			"Has a proverb for everything. Several are about soup.",
			"Challenges staircases to tests of agility.",
			"Counts breathing exercises as a perfectly good hobby.",
			"Applies martial arts terminology to making tea.",
			"Claims the dramatic leap was actually a stretching exercise.",
		},
		Criteria: "inner discipline, agility, mastery of self; fits martial artists, yoga teachers, " +
			"dancers, athletes, acrobats",
	},
	{
		ID:     "paladin",
		Name:   "Paladin",
		Symbol: "⚜️",
		Blurb:  "Swore an oath. Takes it personally.",
		Description: "You made a promise — to a god, a crown, a cause, or simply the idea that " +
			"Tuesday should be more just — and you will keep it even if the universe objects. " +
			"You are the party's shining wall of no when evil shows up, and your smites come with " +
			"a sermon.",
		Traits: []string{"Divine Smite", "Lay on Hands", "Sacred Oath", "Auras"},
		Quirks: [6]string{
			"Swore a solemn oath to never skip breakfast.",
			"Polishes armor while rehearsing inspiring speeches.",
			"Reads every contract aloud with heroic intensity.",
			"Has a very strict code about returning borrowed books.",
			"Gives villains one chance and a surprisingly helpful pamphlet.",
			"Treats keeping small promises like saving kingdoms.",
		},
		Criteria: "oathbound heroism, justice, conviction; fits lawyers, activists, military " +
			"officers, coaches, first responders, anyone with a cause",
	},
	{
		ID:     "ranger",
		Name:   "Ranger",
		Symbol: "🏹",
		Blurb:  "Knows the wilds — and where you hid the cookies.",
		Description: "You are the one who notices the broken twig and what it means. " +
			"Half warrior, half scout, half pet companion (yes, three halves — you are that good). " +
			"Cities make you twitch; the wilderness makes sense. Your wolf is better company than " +
			"most people anyway.",
		Traits: []string{"Favored Enemy", "Hunter's Mark", "Beast Companion", "Trailcraft"},
		Quirks: [6]string{
			"Can identify every footprint except their own muddy ones.",
			"Rates cities by how quickly they can leave them.",
			"Has a trail snack for every possible emergency.",
			"Knows which birds are gossips and never tells them secrets.",
			"Checks the wind before ordering at a tavern.",
			"Calls every long walk a reconnaissance mission.",
		},
		Criteria: "tracking, wilderness lore, survival; fits hunters, outdoors guides, snipers, " +
			"ecologists, dog trainers, farmers",
	},
	{
		ID:     "rogue",
		Name:   "Rogue",
		Symbol: "🗡️",
		Blurb:  "If you saw it, they didn't do it.",
		Description: "Doors are suggestions, locks are hobbies, and fair fight is an oxymoron. " +
			"You live by your wits and other people's wallets. The party calls you morally " +
			"flexible; you call them clients. Behind the smirk is the only person who checked the " +
			"ceiling for traps.",
		Traits: []string{"Sneak Attack", "Expertise", "Evasion", "The Ceiling Is Trapped"},
		Quirks: [6]string{
			"Treats locks as a friendly puzzle competition.",
			"Never enters a room without checking the ceiling.",
			"Knows at least six names. None are the one on the form.",
			"Leaves calling cards with irritatingly good penmanship.",
			"Collects tiny spoons. Refuses to explain their origins.",
			"Has a perfectly legitimate reason for owning that grappling hook.",
		},
		Criteria: "stealth, cunning, dexterity, quick wit; fits magicians, locksmiths, detectives, " +
			"investigators, stage performers, journalists, couriers",
	},
	{
		ID:     "sorcerer",
		Name:   "Sorcerer",
		Symbol: "✨",
		Blurb:  "Magic in the blood. Chaos in the vibes.",
		Description: "You never studied magic — magic picked you, like a cosmic inheritance you " +
			"did not know was in the family. Power hums under your skin and occasionally leaks " +
			"out sideways. Spellbooks are for nerds; you ARE the spellbook, and the footnotes are " +
			"weird.",
		Traits: []string{"Metamagic", "Wild Magic", "Inherited Weirdness", "Charisma Casting"},
		Quirks: [6]string{
			"Accidentally makes their tea sparkle when distracted.",
			"Blames all magical mishaps on the family tree.",
			"Claims glowing eyes are just a hereditary condition.",
			"Has an entirely unearned rivalry with every spellbook.",
			"Apologizes to reality after an enthusiastic spell.",
			"Treats spontaneous levitation as a minor inconvenience.",
		},
		Criteria: "innate power, chaos, raw talent; fits artists, prodigies, performers, " +
			"influencers, anyone commonly described as a lot",
	},
	{
		ID:     "warlock",
		Name:   "Warlock",
		Symbol: "👁️",
		Blurb:  "Great power, suspicious terms and conditions.",
		Description: "You read the fine print and signed anyway. Somewhere out there is your " +
			"Patron — a fiend, an archfey, a Great Old One, something with too many tentacles — " +
			"and they text you assignments at 3am. The power is real, the price is TBD, and your " +
			"party has questions you cannot answer.",
		Traits: []string{"Eldritch Blast", "Pact Magic", "The Patron", "Fine Print"},
		Quirks: [6]string{
			"Reads patron messages but never sends read receipts.",
			"Has a loyalty card for questionable bargains.",
			"Lists 'otherworldly networking' as a professional skill.",
			"Negotiates eldritch favors like a seasoned market haggler.",
			"Keeps a polite calendar reminder for impending doom.",
			"Insists the ominous whispers are just background ambience.",
		},
		Criteria: "forbidden power, patronage, negotiation with the unknown; fits consultants, " +
			"occultists, lawyers of odd contracts, anyone with a mysterious mentor or a " +
			"suspiciously good job offer",
	},
	{
		ID:     "wizard",
		Name:   "Wizard",
		Symbol: "📖",
		Blurb:  "Has a spell for that. And a book about it.",
		Description: "You solved the universe's problems the hard way: reading. " +
			"Your spellbook is your diary, your degree, and your security blanket. You can reshape " +
			"reality with a word — right after you prepare the word in the morning. " +
			"Squishy? Physically. Intellectually? Terrifying.",
		Traits: []string{"Spellbook", "Ritual Casting", "Prepared Spells", "Fireball, Classic"},
		Quirks: [6]string{
			"Color-codes spells and has strong opinions about bookmarks.",
			"Carries three emergency books and one emergency biscuit.",
			"Annotates dungeon walls when the notebook runs out.",
			"Has prepared a spell specifically for reheating tea.",
			"Insists every catastrophe needs a properly cited footnote.",
			"Labels the spellbook 'Do not lend to the bard.'",
		},
		Criteria: "study, arcane knowledge, intellect; fits scholars, scientists, programmers, " +
			"librarians, engineers, grad students, analysts",
	},
}

// ByID looks a class up by its key.
func ByID(id string) (Class, bool) {
	for _, c := range All {
		if c.ID == id {
			return c, true
		}
	}
	return Class{}, false
}

// Criteria returns the Jev option map: class key -> matching hints.
func Criteria() map[string]string {
	m := make(map[string]string, len(All))
	for _, c := range All {
		m[c.ID] = c.Criteria
	}
	return m
}
