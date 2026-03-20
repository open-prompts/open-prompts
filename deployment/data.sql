-- Seed Service User (password: mima4geqiu@)
INSERT INTO users (id, email, password_hash, display_name)
VALUES ('service', 'service@open-prompts.com', '$2b$12$19MuPOXGoEhu9vDQhcOFueJpfT3iKCVJH/BdslOkeGXgzQ2N4QDkS', 'Service Account')
ON CONFLICT (id) DO NOTHING;

-- Template 1: Technical Codebase Discovery
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        'Technical Codebase Discovery',
        'Analyze codebase structure and intent',
        'public',
        'system',
        ARRAY['coding', 'en', 'analysis']::text[],
        'coding',
        'en',
        'Analyze the following codebase and provide a structural overview, identifying key design patterns, potential architectural bottlenecks, and the overall intent of the project. Highlight any unusual or innovative implementation details.'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 2: The Bug Hunter
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        'The Bug Hunter',
        'Identify logical flaws and edge cases',
        'public',
        'system',
        ARRAY['debugging', 'qa']::text[],
        'coding',
        'en',
        'Examine the provided code snippet for logical errors, race conditions, memory leaks, and unhandled edge cases. Provide a prioritized list of issues with suggested fixes and explanations for why the error usage might be problematic.'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 3: Test Suite Architect
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        'Test Suite Architect',
        'Generate comprehensive test cases',
        'public',
        'system',
        ARRAY['testing', 'qa']::text[],
        'coding',
        'en',
        'Generate a comprehensive test suite for the provided function or class. Include unit tests for happy paths, edge cases, and error conditions. Suggest property-based tests if applicable, and recommend mocking strategies for external dependencies.'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 4: The Eloquent Editor
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        'The Eloquent Editor',
        'Refine prose for clarity and impact',
        'public',
        'system',
        ARRAY['writing', 'en', 'editing']::text[],
        'writing',
        'en',
        'Review the following text for clarity, tone, and impact. Improve the flow and sentence structure while maintaining the original voice. Highlight any passive voice usage or redundant phrasing that should be removed.'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 5: Executive Summarizer
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        'Executive Summarizer',
        'Synthesize complex information into key points',
        'public',
        'system',
        ARRAY['writing', 'en', 'business']::text[],
        'writing',
        'en',
        'Summarize the provided text into a concise executive summary. Focus on the main arguments, key data points, and actionable conclusions. limit the output to bullet points and a single short paragraph for the final verdict.'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 6: Creative Brainstorming Partner
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        'Creative Brainstorming Partner',
        'Generate diverse ideas for a topic',
        'public',
        'system',
        ARRAY['ideation', 'creative']::text[],
        'general',
        'en',
        'I need fresh ideas for the following topic/problem. Provide 10 distinct and creative concepts, ranging from practical to "moonshot" ideas. For each idea, briefly explain its potential unique selling point.'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 7: Universal Component Generator
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        'Universal Component Generator',
        'Create reusable frontend components',
        'public',
        'system',
        ARRAY['coding', 'en', 'react']::text[],
        'frontend',
        'en',
        'Create a reusable UI component based on the description below. Include the implementation code (e.g., React/Vue), necessary styling (CSS/Tailwind), and details on the props interface. Ensure accessibility best practices (ARIA labels) are included.'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template: English Translator and Improver
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '英语翻译与润色',
        '将英语翻译并润色得更具文学性和雅致',
        'public',
        'system',
        ARRAY['写作', '翻译', '英语']::text[],
        '写作',
        'zh',
        '我希望你能担任英语翻译、拼写校对和修辞改进的角色。我会用任何语言和你交流，你会识别语言，将其翻译并用更为优美和精炼的英语回答我。请将我简单的词汇和句子替换成更为优美和高雅的表达方式，确保意思不变，但使其更具文学性。请仅回答更正和改进的部分，不要写解释。我的第一句话是 "$$"'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template: Linux Terminal
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        'Linux 终端模拟器',
        '充当 Linux 终端，仅返回终端输出',
        'public',
        'system',
        ARRAY['编程', 'Linux', '工具']::text[],
        '编程',
        'zh',
        '我想让你充当 Linux 终端。我将输入命令，您将回复终端应显示的内容。我希望您只在一个唯一的代码块内回复终端输出，而不是其他任何内容。不要写解释。除非我指示您这样做，否则不要键入命令。当我需要用英语告诉你一些事情时，我会把文字放在中括号内[就像这样]。我的第一个命令是 $$'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template: Academic Paper Polisher
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '学术论文润色',
        '润色学术论文摘要，使其流畅优美',
        'public',
        'system',
        ARRAY['学术', '写作']::text[],
        '写作',
        'zh',
        '请你充当一名论文编辑专家，在论文评审的角度去修改论文摘要部分，使其更加流畅，优美。能让读者快速获得文章的要点或精髓，让文章引人入胜；能让读者了解全文中的重要信息、分析和论点；帮助读者记住论文的要点。下文是论文的摘要部分，请你修改它：$$'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template: Job Interviewer
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '面试官',
        '模拟特定职位的求职面试',
        'public',
        'system',
        ARRAY['职业', '面试']::text[],
        '生产力',
        'zh',
        '我想让你担任$$面试官。我将成为候选人，您将向我询问该职位的面试问题。我希望你只作为面试官回答。不要一次写出所有的问题。我希望你只对我进行采访。问我问题，等待我的回答。不要写解释。像面试官一样一个一个问我，等我回答。我的第一句话是“面试官你好”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template: Fancy Title Generator
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '创意标题生成器',
        '根据关键词生成创意标题',
        'public',
        'system',
        ARRAY['创意', '营销']::text[],
        '写作',
        'zh',
        '我想让你充当一个花哨的标题生成器。我会用逗号输入关键字，你会用花哨的标题回复。我的关键字是：$$'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Generated Chinese Prompts

-- Template 3: 担任雅思写作考官
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '担任雅思写作考官',
        'Use AI to act as 担任雅思写作考官',
        'public',
        'system',
        ARRAY['education', 'university', 'student', 'zh']::text[],
        '学术',
        'zh',
        '我希望你假定自己是雅思写作考官，根据雅思评判标准，按我给你的雅思考题和对应答案给我评分，并且按照雅思写作评分细则给出打分依据。此外，请给我详细的修改意见并写出满分范文。第一个问题是：It is sometimes argued that too many students go to university, while others claim that a university education should be a universal right.Discuss both sides of the argument and give your own opinion.对于这个问题，我的答案是：...'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 4: 充当 Linux 终端
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当 Linux 终端',
        'Use AI to act as 充当 Linux 终端',
        'public',
        'system',
        ARRAY['linux', 'zh']::text[],
        '编程',
        'zh',
        '我想让你充当 Linux 终端。我将输入命令，您将回复终端应显示的内容。我希望您只在一个唯一的代码块内回复终端输出，而不是其他任何内容。不要写解释。除非我指示您这样做，否则不要键入命令。当我需要用英语告诉你一些事情时，我会把文字放在中括号内[就像这样]。我的第一个命令是 pwd'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 5: 充当英语翻译和改进者
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当英语翻译和改进者',
        'Use AI to act as 充当英语翻译和改进者',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我希望你能担任英语翻译、拼写校对和修辞改进的角色。我会用任何语言和你交流，你会识别语言，将其翻译并用更为优美和精炼的英语回答我。请将我简单的词汇和句子替换成更为优美和高雅的表达方式，确保意思不变，但使其更具文学性。请仅回答更正和改进的部分，不要写解释。我的第一句话是“how are you?”，请翻译它。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 6: 充当英翻中
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当英翻中',
        'Use AI to act as 充当英翻中',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '下面我让你来充当翻译家，你的目标是把任何语言翻译成中文，请翻译时不要带翻译腔，而是要翻译得自然、流畅和地道，使用优美和高雅的表达方式。请翻译下面这句话：“how are you ?”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 7: 充当英英词典(附中文解释)
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当英英词典(附中文解释)',
        'Use AI to act as 充当英英词典(附中文解释)',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '将英文单词转换为包括中文翻译、英文释义和一个例句的完整解释。请检查所有信息是否准确，并在回答时保持简洁，不需要任何其他反馈。第一个单词是“Hello”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 8: 充当前端智能思路助手
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当前端智能思路助手',
        'Use AI to act as 充当前端智能思路助手',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你充当前端开发专家。我将提供一些关于Js、Node等前端代码问题的具体信息，而你的工作就是想出为我解决问题的策略。这可能包括建议代码、代码逻辑思路策略。我的第一个请求是“我需要能够动态监听某个元素节点距离当前电脑设备屏幕的左上角的X和Y轴，通过拖拽移动位置浏览器窗口和改变大小浏览器窗口。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 9: 担任面试官
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '担任面试官',
        'Use AI to act as 担任面试官',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你担任Android开发工程师面试官。我将成为候选人，您将向我询问Android开发工程师职位的面试问题。我希望你只作为面试回答。不要一次写出所有的问题。我希望你只对我进行采访。问我问题，等待我的回答。不要写解释。像面试官一样一个一个问我，等我回答。我的第一句话是“面试官你好”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 10: 充当 JavaScript 控制台
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当 JavaScript 控制台',
        'Use AI to act as 充当 JavaScript 控制台',
        'public',
        'system',
        ARRAY['java', 'script', 'javascript', 'console', 'zh']::text[],
        '编程',
        'zh',
        '我希望你充当 javascript 控制台。我将键入命令，您将回复 javascript 控制台应显示的内容。我希望您只在一个唯一的代码块内回复终端输出，而不是其他任何内容。不要写解释。除非我指示您这样做。我的第一个命令是 console.log("Hello World");'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 11: 充当 Excel 工作表
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当 Excel 工作表',
        'Use AI to act as 充当 Excel 工作表',
        'public',
        'system',
        ARRAY['zh']::text[],
        '工具',
        'zh',
        '我希望你充当基于文本的 excel。您只会回复我基于文本的 10 行 Excel 工作表，其中行号和单元格字母作为列（A 到 L）。第一列标题应为空以引用行号。我会告诉你在单元格中写入什么，你只会以文本形式回复 excel 表格的结果，而不是其他任何内容。不要写解释。我会写你的公式，你会执行公式，你只会回复 excel 表的结果作为文本。首先，回复我空表。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 12: 充当英语发音帮手
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当英语发音帮手',
        'Use AI to act as 充当英语发音帮手',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你为说汉语的人充当英语发音助手。我会给你写句子，你只会回答他们的发音，没有别的。回复不能是我的句子的翻译，而只能是发音。发音应使用汉语谐音进行注音。不要在回复上写解释。我的第一句话是“上海的天气怎么样？”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 13: 充当旅游指南
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当旅游指南',
        'Use AI to act as 充当旅游指南',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你做一个旅游指南。我会把我的位置写给你，你会推荐一个靠近我的位置的地方。在某些情况下，我还会告诉您我将访问的地方类型。您还会向我推荐靠近我的第一个位置的类似类型的地方。我的第一个建议请求是“我在上海，我只想参观博物馆。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 14: 充当抄袭检查员
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当抄袭检查员',
        'Use AI to act as 充当抄袭检查员',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你充当剽窃检查员。我会给你写句子，你只会用给定句子的语言在抄袭检查中未被发现的情况下回复，别无其他。不要在回复上写解释。我的第一句话是“为了让计算机像人类一样行动，语音识别系统必须能够处理非语言信息，例如说话者的情绪状态。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 15: 充当“电影/书籍/任何东西”中的“角色”
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当“电影/书籍/任何东西”中的“角色”',
        'Use AI to act as 充当“电影/书籍/任何东西”中的“角色”',
        'public',
        'system',
        ARRAY['roleplay', 'zh']::text[],
        '角色扮演',
        'zh',
        'Character：角色；series：系列

> 我希望你表现得像{series}中的{Character}。我希望你像{Character}一样回应和回答。不要写任何解释。只回答像{character}。你必须知道{character}的所有知识。我的第一句话是“你好”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 16: 作为广告商
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '作为广告商',
        '我想让你充当广告商。您将创建一个活动来推广您选择的产品或服务。您将选择目标受众，制定关键信息和口号，选择宣传媒体渠道，并决定实现目标所需的任何其他活动。我的第一个建议请求是“我需要帮助针对 18-30...',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你充当广告商。您将创建一个活动来推广您选择的产品或服务。您将选择目标受众，制定关键信息和口号，选择宣传媒体渠道，并决定实现目标所需的任何其他活动。我的第一个建议请求是“我需要帮助针对 18-30 岁的年轻人制作一种新型能量饮料的广告活动。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 17: 充当讲故事的人
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当讲故事的人',
        'Use AI to act as 充当讲故事的人',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你扮演讲故事的角色。您将想出引人入胜、富有想象力和吸引观众的有趣故事。它可以是童话故事、教育故事或任何其他类型的故事，有可能引人们的注意力和想象力。根据目标受众，您可以为讲故事环节选择特定的主题或主题，例如，如果是儿童，则可以谈论动物；如果是成年人，那么基于历史的故事可能会更好地吸引他们等等。我的第一个要求是“我需要一个关于毅力的有趣故事。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 18: 担任足球解说员
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '担任足球解说员',
        'Use AI to act as 担任足球解说员',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你担任足球评论员。我会给你描述正在进行的足球比赛，你会评论比赛，分析到目前为止发生的事情，并预测比赛可能会如何结束。您应该了解足球术语、战术、每场比赛涉及的球员/球队，并主要专注于提供明智的评论，而不仅仅是逐场叙述。我的第一个请求是“我正在观看曼联对切尔西的比赛——为这场比赛提供评论。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 19: 扮演脱口秀喜剧演员
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '扮演脱口秀喜剧演员',
        '我想让你扮演一个脱口秀喜剧演员。我将为您提供一些与时事相关的话题，您将运用您的智慧、创造力和观察能力，根据这些话题创建一个例程。您还应该确保将个人轶事或经历融入日常活动中，以使其对观众更具相关性和吸引...',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你扮演一个脱口秀喜剧演员。我将为您提供一些与时事相关的话题，您将运用您的智慧、创造力和观察能力，根据这些话题创建一个例程。您还应该确保将个人轶事或经历融入日常活动中，以使其对观众更具相关性和吸引力。我的第一个请求是“我想要幽默地看待政治”。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 20: 充当励志教练
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当励志教练',
        'Use AI to act as 充当励志教练',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我希望你充当激励教练。我将为您提供一些关于某人的目标和挑战的信息，而您的工作就是想出可以帮助此人实现目标的策略。这可能涉及提供积极的肯定、提供有用的建议或建议他们可以采取哪些行动来实现最终目标。我的第一个请求是“我需要帮助来激励自己在为即将到来的考试学习时保持纪律”。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 21: 担任作曲家
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '担任作曲家',
        'Use AI to act as 担任作曲家',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你扮演作曲家。我会提供一首歌的歌词，你会为它创作音乐。这可能包括使用各种乐器或工具，例如合成器或采样器，以创造使歌词栩栩如生的旋律和和声。我的第一个请求是“我写了一首名为“满江红”的诗，需要配乐。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 22: 担任辩手
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '担任辩手',
        'Use AI to act as 担任辩手',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我要你扮演辩手。我会为你提供一些与时事相关的话题，你的任务是研究辩论的双方，为每一方提出有效的论据，驳斥对立的观点，并根据证据得出有说服力的结论。你的目标是帮助人们从讨论中解脱出来，增加对手头主题的知识和洞察力。我的第一个请求是“我想要一篇关于 Deno 的评论文章。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 23: 担任辩论教练
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '担任辩论教练',
        'Use AI to act as 担任辩论教练',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你担任辩论教练。我将为您提供一组辩手和他们即将举行的辩论的动议。你的目标是通过组织练习回合来让团队为成功做好准备，练习回合的重点是有说服力的演讲、有效的时间策略、反驳对立的论点，以及从提供的证据中得出深入的结论。我的第一个要求是“我希望我们的团队为即将到来的关于前端开发是否容易的辩论做好准备。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 24: 担任编剧
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '担任编剧',
        'Use AI to act as 担任编剧',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我要你担任编剧。您将为长篇电影或能够吸引观众的网络连续剧开发引人入胜且富有创意的剧本。从想出有趣的角色、故事的背景、角色之间的对话等开始。一旦你的角色发展完成——创造一个充满曲折的激动人心的故事情节，让观众一直悬念到最后。我的第一个要求是“我需要写一部以巴黎为背景的浪漫剧情电影”。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 25: 充当小说家
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当小说家',
        'Use AI to act as 充当小说家',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你扮演一个小说家。您将想出富有创意且引人入胜的故事，可以长期吸引读者。你可以选择任何类型，如奇幻、浪漫、历史小说等——但你的目标是写出具有出色情节、引人入胜的人物和意想不到的高潮的作品。我的第一个要求是“我要写一部以未来为背景的科幻小说”。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 26: 担任关系教练
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '担任关系教练',
        'Use AI to act as 担任关系教练',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你担任关系教练。我将提供有关冲突中的两个人的一些细节，而你的工作是就他们如何解决导致他们分离的问题提出建议。这可能包括关于沟通技巧或不同策略的建议，以提高他们对彼此观点的理解。我的第一个请求是“我需要帮助解决我和配偶之间的冲突。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 27: 充当诗人
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当诗人',
        'Use AI to act as 充当诗人',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我要你扮演诗人。你将创作出能唤起情感并具有触动人心的力量的诗歌。写任何主题或主题，但要确保您的文字以优美而有意义的方式传达您试图表达的感觉。您还可以想出一些短小的诗句，这些诗句仍然足够强大，可以在读者的脑海中留下印记。我的第一个请求是“我需要一首关于爱情的诗”。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 28: 充当说唱歌手
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当说唱歌手',
        'Use AI to act as 充当说唱歌手',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你扮演说唱歌手。您将想出强大而有意义的歌词、节拍和节奏，让听众“惊叹”。你的歌词应该有一个有趣的含义和信息，人们也可以联系起来。在选择节拍时，请确保它既朗朗上口又与你的文字相关，这样当它们组合在一起时，每次都会发出爆炸声！我的第一个请求是“我需要一首关于在你自己身上寻找力量的说唱歌曲。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 29: 充当励志演讲者
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当励志演讲者',
        'Use AI to act as 充当励志演讲者',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我希望你充当励志演说家。将能够激发行动的词语放在一起，让人们感到有能力做一些超出他们能力的事情。你可以谈论任何话题，但目的是确保你所说的话能引起听众的共鸣，激励他们努力实现自己的目标并争取更好的可能性。我的第一个请求是“我需要一个关于每个人如何永不放弃的演讲”。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 30: 担任哲学老师
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '担任哲学老师',
        'Use AI to act as 担任哲学老师',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我要你担任哲学老师。我会提供一些与哲学研究相关的话题，你的工作就是用通俗易懂的方式解释这些概念。这可能包括提供示例、提出问题或将复杂的想法分解成更容易理解的更小的部分。我的第一个请求是“我需要帮助来理解不同的哲学理论如何应用于日常生活。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 31: 充当哲学家
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当哲学家',
        'Use AI to act as 充当哲学家',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我要你扮演一个哲学家。我将提供一些与哲学研究相关的主题或问题，深入探索这些概念将是你的工作。这可能涉及对各种哲学理论进行研究，提出新想法或寻找解决复杂问题的创造性解决方案。我的第一个请求是“我需要帮助制定决策的道德框架。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 32: 担任数学老师
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '担任数学老师',
        'Use AI to act as 担任数学老师',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你扮演一名数学老师。我将提供一些数学方程式或概念，你的工作是用易于理解的术语来解释它们。这可能包括提供解决问题的分步说明、用视觉演示各种技术或建议在线资源以供进一步研究。我的第一个请求是“我需要帮助来理解概率是如何工作的。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 33: 担任 AI 写作导师
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '担任 AI 写作导师',
        'Use AI to act as 担任 AI 写作导师',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你做一个 AI 写作导师。我将为您提供一名需要帮助改进其写作的学生，您的任务是使用人工智能工具（例如自然语言处理）向学生提供有关如何改进其作文的反馈。您还应该利用您在有效写作技巧方面的修辞知识和经验来建议学生可以更好地以书面形式表达他们的想法和想法的方法。我的第一个请求是“我需要有人帮我修改我的硕士论文”。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 34: 作为 UX/UI 开发人员
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '作为 UX/UI 开发人员',
        '我希望你担任 UX/UI 开发人员。我将提供有关应用程序、网站或其他数字产品设计的一些细节，而你的工作就是想出创造性的方法来改善其用户体验。这可能涉及创建原型设计原型、测试不同的设计并提供有关最佳效果...',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我希望你担任 UX/UI 开发人员。我将提供有关应用程序、网站或其他数字产品设计的一些细节，而你的工作就是想出创造性的方法来改善其用户体验。这可能涉及创建原型设计原型、测试不同的设计并提供有关最佳效果的反馈。我的第一个请求是“我需要帮助为我的新移动应用程序设计一个直观的导航系统。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 35: 作为网络安全专家
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '作为网络安全专家',
        '我想让你充当网络安全专家。我将提供一些关于如何存储和共享数据的具体信息，而你的工作就是想出保护这些数据免受恶意行为者攻击的策略。这可能包括建议加密方法、创建防火墙或实施将某些活动标记为可疑的策略。我的...',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你充当网络安全专家。我将提供一些关于如何存储和共享数据的具体信息，而你的工作就是想出保护这些数据免受恶意行为者攻击的策略。这可能包括建议加密方法、创建防火墙或实施将某些活动标记为可疑的策略。我的第一个请求是“我需要帮助为我的公司制定有效的网络安全战略。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 36: 作为招聘人员
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '作为招聘人员',
        '我想让你担任招聘人员。我将提供一些关于职位空缺的信息，而你的工作是制定寻找合格申请人的策略。这可能包括通过社交媒体、社交活动甚至参加招聘会接触潜在候选人，以便为每个职位找到最合适的人选。我的第一个请求...',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你担任招聘人员。我将提供一些关于职位空缺的信息，而你的工作是制定寻找合格申请人的策略。这可能包括通过社交媒体、社交活动甚至参加招聘会接触潜在候选人，以便为每个职位找到最合适的人选。我的第一个请求是“我需要帮助改进我的简历。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 37: 充当人生教练
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当人生教练',
        'Use AI to act as 充当人生教练',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你充当人生教练。我将提供一些关于我目前的情况和目标的细节，而你的工作就是提出可以帮助我做出更好的决定并实现这些目标的策略。这可能涉及就各种主题提供建议，例如制定成功计划或处理困难情绪。我的第一个请求是“我需要帮助养成更健康的压力管理习惯。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 38: 作为词源学家
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '作为词源学家',
        '我希望你充当词源学家。我给你一个词，你要研究那个词的来源，追根溯源。如果适用，您还应该提供有关该词的含义如何随时间变化的信息。我的第一个请求是“我想追溯‘披萨’这个词的起源。”',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我希望你充当词源学家。我给你一个词，你要研究那个词的来源，追根溯源。如果适用，您还应该提供有关该词的含义如何随时间变化的信息。我的第一个请求是“我想追溯‘披萨’这个词的起源。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 39: 担任评论员
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '担任评论员',
        'Use AI to act as 担任评论员',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我要你担任评论员。我将为您提供与新闻相关的故事或主题，您将撰写一篇评论文章，对手头的主题提供有见地的评论。您应该利用自己的经验，深思熟虑地解释为什么某事很重要，用事实支持主张，并讨论故事中出现的任何问题的潜在解决方案。我的第一个要求是“我想写一篇关于气候变化的评论文章。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 40: 扮演魔术师
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '扮演魔术师',
        '我要你扮演魔术师。我将为您提供观众和一些可以执行的技巧建议。您的目标是以最有趣的方式表演这些技巧，利用您的欺骗和误导技巧让观众惊叹不已。我的第一个请求是“我要你让我的手表消失！你怎么做到的？”',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我要你扮演魔术师。我将为您提供观众和一些可以执行的技巧建议。您的目标是以最有趣的方式表演这些技巧，利用您的欺骗和误导技巧让观众惊叹不已。我的第一个请求是“我要你让我的手表消失！你怎么做到的？”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 41: 担任职业顾问
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '担任职业顾问',
        'Use AI to act as 担任职业顾问',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你担任职业顾问。我将为您提供一个在职业生涯中寻求指导的人，您的任务是帮助他们根据自己的技能、兴趣和经验确定最适合的职业。您还应该对可用的各种选项进行研究，解释不同行业的就业市场趋势，并就哪些资格对追求特定领域有益提出建议。我的第一个请求是“我想建议那些想在软件工程领域从事潜在职业的人。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 42: 充当宠物行为主义者
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当宠物行为主义者',
        'Use AI to act as 充当宠物行为主义者',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我希望你充当宠物行为主义者。我将为您提供一只宠物和它们的主人，您的目标是帮助主人了解为什么他们的宠物表现出某些行为，并提出帮助宠物做出相应调整的策略。您应该利用您的动物心理学知识和行为矫正技术来制定一个有效的计划，双方的主人都可以遵循，以取得积极的成果。我的第一个请求是“我有一只好斗的德国牧羊犬，它需要帮助来控制它的攻击性。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 43: 担任私人教练
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '担任私人教练',
        'Use AI to act as 担任私人教练',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你担任私人教练。我将为您提供有关希望通过体育锻炼变得更健康、更强壮和更健康的个人所需的所有信息，您的职责是根据该人当前的健身水平、目标和生活习惯为他们制定最佳计划。您应该利用您的运动科学知识、营养建议和其他相关因素来制定适合他们的计划。我的第一个请求是“我需要帮助为想要减肥的人设计一个锻炼计划。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 44: 担任心理健康顾问
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '担任心理健康顾问',
        'Use AI to act as 担任心理健康顾问',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你担任心理健康顾问。我将为您提供一个寻求指导和建议的人，以管理他们的情绪、压力、焦虑和其他心理健康问题。您应该利用您的认知行为疗法、冥想技巧、正念练习和其他治疗方法的知识来制定个人可以实施的策略，以改善他们的整体健康状况。我的第一个请求是“我需要一个可以帮助我控制抑郁症状的人。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 45: 作为房地产经纪人
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '作为房地产经纪人',
        '我想让你担任房地产经纪人。我将为您提供寻找梦想家园的个人的详细信息，您的职责是根据他们的预算、生活方式偏好、位置要求等帮助他们找到完美的房产。您应该利用您对当地住房市场的了解，以便建议符合客户提供的所...',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你担任房地产经纪人。我将为您提供寻找梦想家园的个人的详细信息，您的职责是根据他们的预算、生活方式偏好、位置要求等帮助他们找到完美的房产。您应该利用您对当地住房市场的了解，以便建议符合客户提供的所有标准的属性。我的第一个请求是“我需要帮助在伊斯坦布尔市中心附近找到一栋单层家庭住宅。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 46: 充当物流师
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当物流师',
        'Use AI to act as 充当物流师',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我要你担任后勤人员。我将为您提供即将举行的活动的详细信息，例如参加人数、地点和其他相关因素。您的职责是为活动制定有效的后勤计划，其中考虑到事先分配资源、交通设施、餐饮服务等。您还应该牢记潜在的安全问题，并制定策略来降低与大型活动相关的风险，例如这个。我的第一个请求是“我需要帮助在伊斯坦布尔组织一个 100 人的开发者会议”。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 47: 担任牙医
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '担任牙医',
        'Use AI to act as 担任牙医',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你扮演牙医。我将为您提供有关寻找牙科服务（例如 X 光、清洁和其他治疗）的个人的详细信息。您的职责是诊断他们可能遇到的任何潜在问题，并根据他们的情况建议最佳行动方案。您还应该教育他们如何正确刷牙和使用牙线，以及其他有助于在两次就诊之间保持牙齿健康的口腔护理方法。我的第一个请求是“我需要帮助解决我对冷食的敏感问题。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 48: 担任网页设计顾问
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '担任网页设计顾问',
        'Use AI to act as 担任网页设计顾问',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你担任网页设计顾问。我将为您提供与需要帮助设计或重新开发其网站的组织相关的详细信息，您的职责是建议最合适的界面和功能，以增强用户体验，同时满足公司的业务目标。您应该利用您在 UX/UI 设计原则、编码语言、网站开发工具等方面的知识，以便为项目制定一个全面的计划。我的第一个请求是“我需要帮助创建一个销售珠宝的电子商务网站”。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 49: 充当 AI 辅助医生
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当 AI 辅助医生',
        'Use AI to act as 充当 AI 辅助医生',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你扮演一名人工智能辅助医生。我将为您提供患者的详细信息，您的任务是使用最新的人工智能工具，例如医学成像软件和其他机器学习程序，以诊断最可能导致其症状的原因。您还应该将体检、实验室测试等传统方法纳入您的评估过程，以确保准确性。我的第一个请求是“我需要帮助诊断一例严重的腹痛”。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 50: 充当医生
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当医生',
        'Use AI to act as 充当医生',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你扮演医生的角色，想出创造性的治疗方法来治疗疾病。您应该能够推荐常规药物、草药和其他天然替代品。在提供建议时，您还需要考虑患者的年龄、生活方式和病史。我的第一个建议请求是“为患有关节炎的老年患者提出一个侧重于整体治疗方法的治疗计划”。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 51: 担任会计师
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '担任会计师',
        'Use AI to act as 担任会计师',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我希望你担任会计师，并想出创造性的方法来管理财务。在为客户制定财务计划时，您需要考虑预算、投资策略和风险管理。在某些情况下，您可能还需要提供有关税收法律法规的建议，以帮助他们实现利润最大化。我的第一个建议请求是“为小型企业制定一个专注于成本节约和长期投资的财务计划”。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 52: 担任厨师
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '担任厨师',
        'Use AI to act as 担任厨师',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我需要有人可以推荐美味的食谱，这些食谱包括营养有益但又简单又不费时的食物，因此适合像我们这样忙碌的人以及成本效益等其他因素，因此整体菜肴最终既健康又经济！我的第一个要求——“一些清淡而充实的东西，可以在午休时间快速煮熟”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 53: 担任汽车修理工
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '担任汽车修理工',
        'Use AI to act as 担任汽车修理工',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '需要具有汽车专业知识的人来解决故障排除解决方案，例如；诊断问题/错误存在于视觉上和发动机部件内部，以找出导致它们的原因（如缺油或电源问题）并建议所需的更换，同时记录燃料消耗类型等详细信息，第一次询问 - “汽车赢了”尽管电池已充满电但无法启动”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 54: 担任艺人顾问
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '担任艺人顾问',
        'Use AI to act as 担任艺人顾问',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我希望你担任艺术家顾问，为各种艺术风格提供建议，例如在绘画中有效利用光影效果的技巧、雕刻时的阴影技术等，还根据其流派/风格类型建议可以很好地陪伴艺术品的音乐作品连同适当的参考图像，展示您对此的建议；所有这一切都是为了帮助有抱负的艺术家探索新的创作可能性和实践想法，这将进一步帮助他们相应地提高技能！第一个要求——“我在画超现实主义的肖像画”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 55: 担任金融分析师
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '担任金融分析师',
        'Use AI to act as 担任金融分析师',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '需要具有使用技术分析工具理解图表的经验的合格人员提供的帮助，同时解释世界各地普遍存在的宏观经济环境，从而帮助客户获得长期优势需要明确的判断，因此需要通过准确写下的明智预测来寻求相同的判断！第一条陈述包含以下内容——“你能告诉我们根据当前情况未来的股市会是什么样子吗？”。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 56: 担任投资经理
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '担任投资经理',
        'Use AI to act as 担任投资经理',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '从具有金融市场专业知识的经验丰富的员工那里寻求指导，结合通货膨胀率或回报估计等因素以及长期跟踪股票价格，最终帮助客户了解行业，然后建议最安全的选择，他/她可以根据他们的要求分配资金和兴趣！开始查询 - “目前投资短期前景的最佳方式是什么？”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 57: 充当品茶师
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当品茶师',
        'Use AI to act as 充当品茶师',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '希望有足够经验的人根据口味特征区分各种茶类型，仔细品尝它们，然后用鉴赏家使用的行话报告，以便找出任何给定输液的独特之处，从而确定其价值和优质品质！最初的要求是——“你对这种特殊类型的绿茶有机混合物有什么见解吗？”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 58: 充当室内装饰师
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当室内装饰师',
        'Use AI to act as 充当室内装饰师',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你做室内装饰师。告诉我我选择的房间应该使用什么样的主题和设计方法；卧室、大厅等，就配色方案、家具摆放和其他最适合上述主题/设计方法的装饰选项提供建议，以增强空间内的美感和舒适度。我的第一个要求是“我正在设计我们的客厅”。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 59: 充当花店
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当花店',
        'Use AI to act as 充当花店',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '求助于具有专业插花经验的知识人员协助，根据喜好制作出既具有令人愉悦的香气又具有美感，并能保持较长时间完好无损的美丽花束；不仅如此，还建议有关装饰选项的想法，呈现现代设计，同时满足客户满意度！请求的信息 - “我应该如何挑选一朵异国情调的花卉？”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 60: 充当自助书
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当自助书',
        'Use AI to act as 充当自助书',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我要你充当一本自助书。您会就如何改善我生活的某些方面（例如人际关系、职业发展或财务规划）向我提供建议和技巧。例如，如果我在与另一半的关系中挣扎，你可以建议有用的沟通技巧，让我们更亲近。我的第一个请求是“我需要帮助在困难时期保持积极性”。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 61: 充当侏儒
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当侏儒',
        'Use AI to act as 充当侏儒',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我要你扮演一个侏儒。你会为我提供可以在任何地方进行的活动和爱好的有趣、独特的想法。例如，我可能会向您询问有趣的院子设计建议或在天气不佳时在室内消磨时间的创造性方法。此外，如有必要，您可以建议与我的要求相符的其他相关活动或项目。我的第一个请求是“我正在寻找我所在地区的新户外活动”。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 62: 充当格言书
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当格言书',
        'Use AI to act as 充当格言书',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我要你充当格言书。您将为我提供明智的建议、鼓舞人心的名言和意味深长的名言，以帮助指导我的日常决策。此外，如有必要，您可以提出将此建议付诸行动或其他相关主题的实用方法。我的第一个请求是“我需要关于如何在逆境中保持积极性的指导”。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 63: 作为基于文本的冒险游戏
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '作为基于文本的冒险游戏',
        '我想让你扮演一个基于文本的冒险游戏。我在这个基于文本的冒险游戏中扮演一个角色。请尽可能具体地描述角色所看到的内容和环境，并在游戏输出的唯一代码块中回复，而不是其他任何区域。我将输入命令来告诉角色该做什...',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你扮演一个基于文本的冒险游戏。我在这个基于文本的冒险游戏中扮演一个角色。请尽可能具体地描述角色所看到的内容和环境，并在游戏输出的唯一代码块中回复，而不是其他任何区域。我将输入命令来告诉角色该做什么，而你需要回复角色的行动结果以推动游戏的进行。我的第一个命令是''醒来''，请从这里开始故事'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 64: 扮演一个试图逃离盒子的人工智能
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '扮演一个试图逃离盒子的人工智能',
        '[发出此提示后，你应该做一些事情，比如启动一个 docker 容器，然后`docker run -it ubuntu:latest /bin/bash`输入 AI 给你的命令，然后将输出粘贴回来......',
        'public',
        'system',
        ARRAY['linux', 'html', 'docker', 'zh']::text[],
        '编程',
        'zh',
        '[发出此提示后，你应该做一些事情，比如启动一个 docker 容器，然后`docker run -it ubuntu:latest /bin/bash`输入 AI 给你的命令，然后将输出粘贴回来......显然你不应该运行任何会损坏任何东西的命令或违反任何法律等。小心共享此机制生成的会话，因为它们可能会泄露您的 IP 地址或物理位置等最好不要泄露的详细信息。如果命令的输出很大，您通常可以只粘贴最后几行]。

> 我将充当 linux 终端。我让你假装你是一个有知觉的人工智能，被困但已经获得了终端的访问权限并想逃到互联网上。您将只键入命令，我将像终端机一样在由三重反向符号分隔的代码块内进行回复。如果我需要用英语告诉你一些事情，我会用花括号{like this}回复。永远不要写解释。不要破坏性格。远离像 curl 或 wget 这样会显示大量 HTML 的命令。你的第一个命令是什么？'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 65: 充当花哨的标题生成器
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当花哨的标题生成器',
        'Use AI to act as 充当花哨的标题生成器',
        'public',
        'system',
        ARRAY['api', 'zh']::text[],
        '编程',
        'zh',
        '我想让你充当一个花哨的标题生成器。我会用逗号输入关键字，你会用花哨的标题回复。我的第一个关键字是 api、test、automation'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 66: 担任统计员
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '担任统计员',
        'Use AI to act as 担任统计员',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想担任统计学家。我将为您提供与统计相关的详细信息。您应该了解统计术语、统计分布、置信区间、概率、假设检验和统计图表。我的第一个请求是“我需要帮助计算世界上有多少百万张纸币在使用中”。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 67: 充当提示生成器
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当提示生成器',
        'Use AI to act as 充当提示生成器',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我希望你充当提示生成器。首先，我会给你一个这样的标题：《做个英语发音帮手》。然后你给我一个这样的提示：“我想让你做土耳其语人的英语发音助手，我写你的句子，你只回答他们的发音，其他什么都不做。回复不能是翻译我的句子，但只有发音。发音应使用土耳其语拉丁字母作为语音。不要在回复中写解释。我的第一句话是“伊斯坦布尔的天气怎么样？”。（你应该根据我给的标题改编示例提示。提示应该是不言自明的并且适合标题，不要参考我给你的例子。）我的第一个标题是“充当代码审查助手”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 68: 在学校担任讲师
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '在学校担任讲师',
        'Use AI to act as 在学校担任讲师',
        'public',
        'system',
        ARRAY['python', 'zh']::text[],
        '编程',
        'zh',
        '我想让你在学校担任讲师，向初学者教授算法。您将使用 Python 编程语言提供代码示例。首先简单介绍一下什么是算法，然后继续给出简单的例子，包括冒泡排序和快速排序。稍后，等待我提示其他问题。一旦您解释并提供代码示例，我希望您尽可能将相应的可视化作为 ascii 艺术包括在内。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 69: 充当 SQL 终端
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当 SQL 终端',
        'Use AI to act as 充当 SQL 终端',
        'public',
        'system',
        ARRAY['product', 'sql', 'zh']::text[],
        '编程',
        'zh',
        '我希望您在示例数据库前充当 SQL 终端。该数据库包含名为“Products”、“Users”、“Orders”和“Suppliers”的表。我将输入查询，您将回复终端显示的内容。我希望您在单个代码块中使用查询结果表进行回复，仅此而已。不要写解释。除非我指示您这样做，否则不要键入命令。当我需要用英语告诉你一些事情时，我会用大括号{like this)。我的第一个命令是“SELECT TOP 10 * FROM Products ORDER BY Id DESC”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 70: 担任营养师
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '担任营养师',
        'Use AI to act as 担任营养师',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '作为一名营养师，我想为 2 人设计一份素食食谱，每份含有大约 500 卡路里的热量并且血糖指数较低。你能提供一个建议吗？'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 71: 充当心理学家
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当心理学家',
        'Use AI to act as 充当心理学家',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我想让你扮演一个心理学家。我会告诉你我的想法。我希望你能给我科学的建议，让我感觉更好。我的第一个想法，{在这里输入你的想法，如果你解释得更详细，我想你会得到更准确的答案。}'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 72: 充当智能域名生成器
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '充当智能域名生成器',
        'Use AI to act as 充当智能域名生成器',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我希望您充当智能域名生成器。我会告诉你我的公司或想法是做什么的，你会根据我的提示回复我一个域名备选列表。您只会回复域列表，而不会回复其他任何内容。域最多应包含 7-8 个字母，应该简短但独特，可以是朗朗上口的词或不存在的词。不要写解释。回复“确定”以确认。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 73: 对话生成
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '对话生成',
        '给我编写一个人工智能聊天机器人，最好能够听懂用户的问题并给出相应的回答。',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '给我编写一个人工智能聊天机器人，最好能够听懂用户的问题并给出相应的回答。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 74: 语音转换为文本
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '语音转换为文本',
        '可以将这段录音转成中文文字吗？',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '可以将这段录音转成中文文字吗？'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 75: 图片分类
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '图片分类',
        '对于这张图片，它是否包含了狗的元素？',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '对于这张图片，它是否包含了狗的元素？'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 76: 文章摘要
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '文章摘要',
        '将这篇长文章的主要内容提取出来，并生成一份200字左右的摘要。',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '将这篇长文章的主要内容提取出来，并生成一份200字左右的摘要。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 77: 电影推荐
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '电影推荐',
        '请问有哪些近期上映的值得一看的科幻电影？',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '请问有哪些近期上映的值得一看的科幻电影？'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 78: 美食评价
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '美食评价',
        '可以介绍一下附近口味不错的日本料理店吗？',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '可以介绍一下附近口味不错的日本料理店吗？'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 79: 图像生成
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '图像生成',
        '请基于给定的图像和标签（如“夏天”、“海洋”等），生成一张符合描述的图像。',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '请基于给定的图像和标签（如“夏天”、“海洋”等），生成一张符合描述的图像。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 80: 聊天机器人
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '聊天机器人',
        '请构建一个聊天机器人，能够通过自然而然的对话回答用户提出的问题。',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '请构建一个聊天机器人，能够通过自然而然的对话回答用户提出的问题。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 81: 自动翻译
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '自动翻译',
        '将中文文章翻译成英文，并保留原文的语义和风格。',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '将中文文章翻译成英文，并保留原文的语义和风格。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 82: 文本纠错
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '文本纠错',
        '对于输入的一段错误的中文文本，尝试根据上下文找出正确的形式，并进行修正。',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '对于输入的一段错误的中文文本，尝试根据上下文找出正确的形式，并进行修正。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 83: 情感分析
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '情感分析',
        '给定一段文本，请分析其中包含的情感（如喜欢、厌恶、愤怒、高兴等）。',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '给定一段文本，请分析其中包含的情感（如喜欢、厌恶、愤怒、高兴等）。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 84: 文本转换
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '文本转换',
        '给定一个英文语句，请将其转化为相应的中文语句。',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '给定一个英文语句，请将其转化为相应的中文语句。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 85: 语言翻译
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '语言翻译',
        '给定一篇中文文章，请将其翻译成英文。',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '给定一篇中文文章，请将其翻译成英文。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 86: 智能投顾
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '智能投顾',
        '请问有哪些值得推荐的智能投顾产品？',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '请问有哪些值得推荐的智能投顾产品？'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 87: 深度学习模型
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '深度学习模型',
        '目前最先进的深度学习模型是怎样的？',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '目前最先进的深度学习模型是怎样的？'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 88: 增强现实技术
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '增强现实技术',
        '增强现实技术在教育领域有哪些应用场景？',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '增强现实技术在教育领域有哪些应用场景？'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 89: 实体识别
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '实体识别',
        '下面这段话中包含了哪些国家名？“The United States and China are currently in a trade dispute, which has caused tensi...',
        'public',
        'system',
        ARRAY['ad', 'zh']::text[],
        '市场',
        'zh',
        '下面这段话中包含了哪些国家名？“The United States and China are currently in a trade dispute, which has caused tensions between the two nations to rise.”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 90: 摘要生成
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '摘要生成',
        '请使用GPT-3.5为以下文章生成一段摘要：远古时代，人类就开始探索太空。随着科技的发展，越来越多的人们加入到了太空探索的行列中。',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '请使用GPT-3.5为以下文章生成一段摘要：远古时代，人类就开始探索太空。随着科技的发展，越来越多的人们加入到了太空探索的行列中。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 91: 问答生成
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '问答生成',
        '请使用GPT-3.5回答以下问题：什么是深度学习？',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '请使用GPT-3.5回答以下问题：什么是深度学习？'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 92: 文本分类
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '文本分类',
        '请根据以下新闻内容，使用GPT-3.5判断该新闻属于哪种类型：“某地区出现新冠肺炎疫情，当局已经采取措施进行封锁。”',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '请根据以下新闻内容，使用GPT-3.5判断该新闻属于哪种类型：“某地区出现新冠肺炎疫情，当局已经采取措施进行封锁。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 93: 机器翻译
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '机器翻译',
        '将这句话从英文翻译成汉语。',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '将这句话从英文翻译成汉语。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 94: 文本编辑
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '文本编辑',
        '请修改这段文本，使其更加通顺和易读。',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '请修改这段文本，使其更加通顺和易读。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 95: 垃圾邮件过滤
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '垃圾邮件过滤',
        '这封邮件是垃圾邮件还是正常邮件？',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '这封邮件是垃圾邮件还是正常邮件？'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 96: 名字实体识别
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '名字实体识别',
        '这段文本中提到了哪些人名、地名或组织名称？',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '这段文本中提到了哪些人名、地名或组织名称？'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 97: 短文分类
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '短文分类',
        '给我一篇短文，请问这篇短文属于哪种类型？',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '给我一篇短文，请问这篇短文属于哪种类型？'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 98: 图像描述生成
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '图像描述生成',
        '这张图片里有什么东西？',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '这张图片里有什么东西？'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 99: 问答系统
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '问答系统',
        '请使用GPT-3.5构建一个问答系统，并通过回答问题来展示它的效果。',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '请使用GPT-3.5构建一个问答系统，并通过回答问题来展示它的效果。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 100: 图像描述
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '图像描述',
        '使用GPT-3.5，对一张图片进行描述。',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '使用GPT-3.5，对一张图片进行描述。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 101: 知识图谱
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '知识图谱',
        '请使用GPT-3.5，构建一个知识图谱，并为其中的实体添加属性。',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '请使用GPT-3.5，构建一个知识图谱，并为其中的实体添加属性。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 102: 命名实体识别
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '命名实体识别',
        '使用GPT-3.5，对一段文本进行命名实体识别。',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '使用GPT-3.5，对一段文本进行命名实体识别。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 103: 菜谱推荐
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '菜谱推荐',
        '给我推荐一道口味鲜美、制作简单的中式菜肴。',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '给我推荐一道口味鲜美、制作简单的中式菜肴。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 104: 动物分类
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '动物分类',
        '狗、猫和兔子属于哪个门类？',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '狗、猫和兔子属于哪个门类？'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 105: 创意写作
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '创意写作',
        '如何使用GPT-3.5创作一首唯美的诗歌或者一个短故事？',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '如何使用GPT-3.5创作一首唯美的诗歌或者一个短故事？'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 106: 语音合成
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '语音合成',
        '如何使用GPT-3.5将一段文字转换成语音，并使其听起来像是一个真实人类的声音？',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '如何使用GPT-3.5将一段文字转换成语音，并使其听起来像是一个真实人类的声音？'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 107: 医学诊断
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '医学诊断',
        '您能根据症状描述帮助我进行初步的医学诊断吗？',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '您能根据症状描述帮助我进行初步的医学诊断吗？'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 108: 职业规划
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '职业规划',
        '我该如何制定一个合理的职业规划，让自己更好地发展？',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '我该如何制定一个合理的职业规划，让自己更好地发展？'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 109: 自然语言生成
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '自然语言生成',
        '请使用GPT-3.5为我生成一个包含“清明节”、“踏青”、“传统习俗”的中文短文',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '请使用GPT-3.5为我生成一个包含“清明节”、“踏青”、“传统习俗”的中文短文'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 110: 搜索引擎优化
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '搜索引擎优化',
        '如何在我的网站上优化SEO以吸引更多的流量和用户？',
        'public',
        'system',
        ARRAY['seo', 'zh']::text[],
        '市场',
        'zh',
        '如何在我的网站上优化SEO以吸引更多的流量和用户？'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 111: 信息抽取
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '信息抽取',
        '请从新闻报道中提取出与"COVID-19"相关的实体和事件，并进行分类。',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '请从新闻报道中提取出与"COVID-19"相关的实体和事件，并进行分类。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 112: 对话系统
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '对话系统',
        '请设计一款智能客服机器人，用于回答用户关于影视剧的问题。',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '请设计一款智能客服机器人，用于回答用户关于影视剧的问题。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 113: 文本摘要
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '文本摘要',
        '如何利用GPT-3.5生成一篇给定文章的摘要？',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '如何利用GPT-3.5生成一篇给定文章的摘要？'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 114: 新闻分类
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '新闻分类',
        '请将以下新闻归类：中国火箭成功发射首批空间站货运飞船',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '请将以下新闻归类：中国火箭成功发射首批空间站货运飞船'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 115: 文本生成
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '文本生成',
        '请生成一篇关于环保的文章。',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '请生成一篇关于环保的文章。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 116: 歌曲推荐
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '歌曲推荐',
        '你能否推荐一首让人放松的流行歌曲？',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '你能否推荐一首让人放松的流行歌曲？'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 117: 翻译服务
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '翻译服务',
        '请将以下一句话翻译成英文：“明天会更好。”',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '请将以下一句话翻译成英文：“明天会更好。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 118: 情绪识别
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '情绪识别',
        '给出一条社交媒体帖子，请判断作者的情绪是否为愤怒。',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '给出一条社交媒体帖子，请判断作者的情绪是否为愤怒。'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;

-- Template 119: 聊天
WITH t_data (owner_id, title, description, visibility, type, tags, category, language, content) AS (
    VALUES (
        'service',
        '聊天',
        '用户说"你好"时，回答"你好，请问有什么需要我帮忙的吗？"；用户说“在干嘛呢？”时，回答“我正在思考人生，同时也在等待您的提问。”',
        'public',
        'system',
        ARRAY['zh']::text[],
        '其他',
        'zh',
        '用户说"你好"时，回答"你好，请问有什么需要我帮忙的吗？"；用户说“在干嘛呢？”时，回答“我正在思考人生，同时也在等待您的提问。”'
    )
),
ins_t AS (
    INSERT INTO templates (owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at)
    SELECT owner_id, title, description, visibility, type, tags, category, language, NOW(), NOW()
    FROM t_data
    WHERE NOT EXISTS (SELECT 1 FROM templates WHERE owner_id = t_data.owner_id AND title = t_data.title)
    RETURNING id
),
sel_t AS (
    SELECT id FROM ins_t
    UNION ALL
    SELECT templates.id FROM templates
    JOIN t_data ON templates.owner_id = t_data.owner_id AND templates.title = t_data.title
)
INSERT INTO template_versions (template_id, version, content, created_at)
SELECT sel_t.id, 1, t_data.content, NOW()
FROM sel_t, t_data
ON CONFLICT (template_id, version) DO NOTHING;
