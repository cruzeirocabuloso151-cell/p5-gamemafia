"""
catalog.py — Categorias de ML e temas visuais selecionáveis.

Cada categoria tem uma descrição técnica que é injetada nos prompts
dos agentes para orientar o design do game.
"""

CATEGORIES: dict[str, str] = {
    "Redes Neurais": (
        "Redes neurais artificiais: camadas de neurônios, pesos, funções de "
        "ativação, forward pass e backpropagation. No game, o jogador deve VER "
        "a rede — topologia, ativações se propagando, pesos mudando durante o "
        "treinamento — e interagir diretamente com ela."
    ),
    "Reinforcement Learning": (
        "Aprendizado por reforço: agente, ambiente, estados, ações, recompensas, "
        "política, exploração vs. exploração (epsilon-greedy), Q-learning e "
        "policy gradients. O jogador deve moldar recompensas e observar o agente "
        "aprender por tentativa e erro em tempo real."
    ),
    "Visão Computacional": (
        "Visão computacional: convoluções, filtros/kernels, mapas de features, "
        "detecção de objetos, segmentação e classificação de imagens. O jogador "
        "deve ver o que a máquina 'enxerga' — feature maps, bounding boxes, "
        "camadas convolucionais em ação."
    ),
    "NLP/Transformers": (
        "Processamento de linguagem natural e Transformers: tokens, embeddings, "
        "mecanismo de atenção (attention), contexto e geração de texto. O jogador "
        "deve visualizar atenção entre palavras, embeddings como espaço "
        "geométrico e a construção de sentido token a token."
    ),
    "Algoritmos Genéticos": (
        "Algoritmos genéticos e computação evolutiva: população, fitness, "
        "seleção, crossover, mutação e gerações. O jogador deve criar pressão "
        "seletiva e assistir criaturas/soluções evoluírem visivelmente ao longo "
        "das gerações."
    ),
    "GANs/Geração": (
        "Redes generativas adversariais e modelos generativos: gerador vs. "
        "discriminador, espaço latente, interpolação e síntese de conteúdo. O "
        "jogador deve navegar o espaço latente e participar do duelo "
        "gerador/discriminador como mecânica central."
    ),
    "Clustering/Anomalias": (
        "Aprendizado não-supervisionado: clustering (k-means, DBSCAN), redução "
        "de dimensionalidade e detecção de anomalias. O jogador deve agrupar, "
        "separar e caçar outliers em dados visualizados espacialmente."
    ),
    "Transfer Learning": (
        "Transfer learning e fine-tuning: reaproveitar conhecimento de um "
        "domínio em outro, congelar/descongelar camadas, adaptação de modelos. "
        "O jogador deve transplantar 'cérebros' treinados entre contextos e "
        "lidar com o que transfere bem ou mal."
    ),
}

THEMES: dict[str, str] = {
    "Sci-Fi": "Ficção científica: laboratórios high-tech, hologramas, IA de bordo, estética limpa e futurista.",
    "Medieval Fantasia": "Fantasia medieval: magia como metáfora para ML, grimórios, criaturas, reinos e masmorras.",
    "Cyberpunk": "Cyberpunk: neon, megacorporações, hackers, implantes, chuva e ruas escuras de metrópoles.",
    "Natureza": "Natureza: ecossistemas, florestas, simbiose, crescimento orgânico e ciclos naturais.",
    "Espaço": "Espaço: exploração interestelar, naves, planetas desconhecidos, vazio cósmico e descoberta.",
    "Pós-Apocalipse": "Pós-apocalipse: ruínas, escassez, reconstrução, tecnologia resgatada e sobrevivência.",
    "Subaquático": "Mundo subaquático: oceanos profundos, bioluminescência, pressão, criaturas abissais.",
    "Steampunk": "Steampunk: engrenagens, vapor, latão, autômatos vitorianos e máquinas analíticas.",
}
